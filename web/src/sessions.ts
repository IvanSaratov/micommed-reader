import { Reader } from "./client";
import { DayCharts } from "./charts";
import type { SessionIndex, SessionView, SleepSession } from "./types";

const el = <T extends HTMLElement = HTMLElement>(id: string) =>
  document.getElementById(id) as T;
const visible = (id: string, show: boolean) => {
  el(id).hidden = !show;
};
const button = (id: string) => el<HTMLButtonElement>(id);
const number = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 });
const date = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
});
function dates(s: SleepSession) {
  return date.formatRange(
    new Date(s.start_device_time + "Z"),
    new Date(s.end_device_time + "Z"),
  );
}
function hours(minutes: number) {
  const n = Math.round(minutes);
  if (!n) return "<1 мин";
  const h = Math.floor(n / 60),
    m = n % 60;
  return [h ? h + " ч" : "", m ? m + " мин" : ""].filter(Boolean).join(" ");
}

export class SessionScreen {
  private reader?: Reader;
  private index?: SessionIndex;
  private data?: SessionView;
  private charts?: DayCharts;
  private generation = 0;
  private detailRequest = 0;
  constructor() {
    el<HTMLSelectElement>("sleep-select").onchange = () =>
      void this.select(Number(el<HTMLSelectElement>("sleep-select").value));
    button("session-prev").onclick = () => this.move(-1);
    button("session-next").onclick = () => this.move(1);
    button("session-reset").onclick = () => this.charts?.reset();
    el("session-settings").onsubmit = (e) => {
      e.preventDefault();
      void this.regroup(
        Number(el<HTMLInputElement>("session-gap").value),
      ).catch((error) => {
        el("session-error").textContent = (error as Error).message;
        visible("session-error", true);
      });
    };
  }
  clear() {
    this.generation++;
    this.detailRequest++;
    this.reader = undefined;
    this.index = undefined;
    this.data = undefined;
    this.charts?.clear();
    for (const id of [
      "sleep-select",
      "sleep-stats",
      "sleep-title",
      "sleep-time",
      "session-error",
    ])
      el(id).replaceChildren();
    for (const id of [
      "sleep-result",
      "sleep-empty",
      "session-warning",
      "session-error",
    ])
      visible(id, false);
    el<HTMLInputElement>("session-gap").value = "60";
    for (const id of ["session-prev", "session-next"])
      button(id).disabled = true;
  }
  async load(reader: Reader) {
    this.reader = reader;
    await this.regroup(60);
  }
  private async regroup(gap: number) {
    if (!this.reader) return;
    const token = ++this.generation;
    this.detailRequest++;
    this.data = undefined;
    this.charts?.clear();
    visible("sleep-result", false);
    visible("sleep-empty", false);
    visible("session-error", false);
    el<HTMLSelectElement>("sleep-select").disabled = true;
    button("session-prev").disabled = true;
    button("session-next").disabled = true;
    try {
      const index = await this.reader.call<SessionIndex>("sessions", { gap });
      if (token !== this.generation) return;
      this.index = index;
      const select = el<HTMLSelectElement>("sleep-select");
      select.replaceChildren();
      for (const s of index.sessions) {
        const option = document.createElement("option");
        option.value = String(s.id);
        option.textContent =
          dates(s) +
          " · " +
          s.start_device_time.slice(11, 16) +
          " — " +
          s.end_device_time.slice(11, 16);
        select.append(option);
      }
      select.disabled = !index.sessions.length;
      visible("session-warning", index.issues.length > 0);
      visible("sleep-empty", index.sessions.length === 0);
      if (index.sessions.length) await this.select(index.sessions.at(-1)!.id);
    } catch (error) {
      if (token === this.generation) throw error;
    }
  }
  private move(offset: number) {
    const list = this.index?.sessions || [],
      id = Number(el<HTMLSelectElement>("sleep-select").value);
    const i = list.findIndex((s) => s.id === id) + offset;
    if (list[i]) void this.select(list[i].id);
  }
  private async select(id: number) {
    if (!this.reader || !this.index) return;
    const s = this.index.sessions.find((s) => s.id === id);
    if (!s) return;
    const version = this.generation,
      request = ++this.detailRequest;
    this.data = undefined;
    this.charts?.clear();
    el<HTMLSelectElement>("sleep-select").value = String(id);
    const position = this.index.sessions.indexOf(s);
    button("session-prev").disabled = position === 0;
    button("session-next").disabled =
      position === this.index.sessions.length - 1;
    el("sleep-title").textContent = dates(s);
    el("sleep-time").textContent =
      s.start_device_time.slice(11, 16) +
      " — " +
      s.end_device_time.slice(11, 16);
    const stats = el("sleep-stats");
    stats.replaceChildren();
    const count = ["102", "103", "104"].reduce(
      (n, code) => n + (s.event_counts[code] || 0),
      0,
    );
    const values = [
      [
        "Продолжительность записи",
        hours(s.span_minutes),
        "включая короткие перерывы",
      ],
      ["Дыхательные события", String(count), "за этот сеанс"],
      [
        "AHI",
        s.recalculated_ahi.value === null
          ? "—"
          : number.format(s.recalculated_ahi.value),
        s.recalculated_ahi.value !== null
          ? "событий в час"
          : s.recalculated_ahi.reason === "partial_day_duration"
            ? "есть только суточная длительность"
            : "недостаточно данных",
      ],
    ];
    for (const [name, value, note] of values) {
      const div = document.createElement("div"),
        dt = document.createElement("dt"),
        dd = document.createElement("dd"),
        small = document.createElement("small");
      dt.textContent = name;
      dd.textContent = value;
      small.textContent = note;
      dd.append(small);
      div.append(dt, dd);
      stats.append(div);
    }
    visible("sleep-result", true);
    visible("session-error", false);
    try {
      const data = await this.reader.call<SessionView>("session", {
        session_id: id,
      });
      // Смена набора или порога аннулирует результат прежнего запроса.
      if (version !== this.generation || request !== this.detailRequest) return;
      this.data = data;
      this.draw();
    } catch (error) {
      if (version === this.generation && request === this.detailRequest) {
        el("session-error").textContent = (error as Error).message;
        visible("session-error", true);
      }
    }
  }
  private draw() {
    if (!this.data) return;
    this.charts ||= new DayCharts(el("session-charts"));
    this.charts.show(this.data, {
      simple: true,
      origin: this.data.start_device_time,
      span: this.data.span_minutes,
    });
  }
}
