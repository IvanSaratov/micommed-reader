import { Reader } from "./client";
import { DayCharts } from "./charts";
import { SessionCalendar } from "./session-calendar";
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
function times(s: SleepSession) {
  return (
    s.start_device_time.slice(11, 16) + " — " + s.end_device_time.slice(11, 16)
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
  private selectedId?: number;
  private selectedDay = "";
  private calendar = new SessionCalendar((day, sessions) => {
    const session =
      sessions.find((s) => s.id === this.selectedId) || sessions.at(-1)!;
    void this.select(session.id, day);
  });
  private generation = 0;
  private detailRequest = 0;
  private reasons: Record<string, string> = {};
  constructor() {
    el<HTMLSelectElement>("sleep-select").onchange = () =>
      void this.select(
        Number(el<HTMLSelectElement>("sleep-select").value),
        this.selectedDay,
      );
    button("session-prev").onclick = () => this.move(-1);
    button("session-next").onclick = () => this.move(1);
    button("session-reset").onclick = () => this.charts?.reset();
  }
  clear() {
    this.generation++;
    this.detailRequest++;
    this.reader = undefined;
    this.index = undefined;
    this.selectedId = undefined;
    this.selectedDay = "";
    this.calendar.clear();
    this.data = undefined;
    this.reasons = {};
    this.charts?.clear();
    for (const id of [
      "sleep-select",
      "sleep-stats",
      "sleep-title",
      "sleep-time",
      "session-error",
      "session-warning",
    ])
      el(id).replaceChildren();
    for (const id of [
      "sleep-result",
      "day-sessions",
      "sleep-empty",
      "session-warning",
      "session-error",
    ])
      visible(id, false);
    for (const id of ["session-prev", "session-next"])
      button(id).disabled = true;
  }
  async load(reader: Reader, reasons: Record<string, string>) {
    this.reader = reader;
    this.reasons = reasons;
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
      this.calendar.load(index.sessions);
      const reasons = [
        ...new Set(
          index.issues.map(
            (issue) =>
              this.reasons[issue.code] || "запись не удалось прочитать",
          ),
        ),
      ];
      el("session-warning").textContent = reasons.length
        ? "Часть записей пропущена: " + reasons.join("; ") + "."
        : "";
      visible("session-warning", index.issues.length > 0);
      visible("sleep-empty", index.sessions.length === 0);
      if (index.sessions.length) await this.select(index.sessions.at(-1)!.id);
    } catch (error) {
      if (token === this.generation) throw error;
    }
  }
  private move(offset: number) {
    const list = this.index?.sessions || [],
      id = this.selectedId;
    const i = list.findIndex((s) => s.id === id) + offset;
    if (list[i]) void this.select(list[i].id);
  }
  private async select(id: number, day?: string) {
    if (!this.reader || !this.index) return;
    const s = this.index.sessions.find((s) => s.id === id);
    if (!s) return;
    this.selectedDay = day ?? s.start_device_time.slice(0, 10);
    const sessions = this.calendar.select(this.selectedDay);
    const select = el<HTMLSelectElement>("sleep-select");
    select.replaceChildren(
      ...sessions.map((item) => new Option(times(item), String(item.id))),
    );
    select.value = String(id);
    select.disabled = sessions.length < 2;
    visible("day-sessions", sessions.length > 1);
    if (this.selectedId === id && this.data) return;
    this.selectedId = id;
    const version = this.generation,
      request = ++this.detailRequest;
    this.data = undefined;
    this.charts?.clear();
    const position = this.index.sessions.indexOf(s);
    button("session-prev").disabled = position === 0;
    button("session-next").disabled =
      position === this.index.sessions.length - 1;
    el("sleep-title").textContent = dates(s);
    el("sleep-time").textContent = times(s);
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
      ["Апноэ и гипопноэ", String(count), "за этот сеанс"],
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
