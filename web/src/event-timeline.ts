import type { Event as DeviceEvent } from "./types";

const explanations: Record<number, string> = {
  102: "Пауза в дыхании, при которой воздух перестаёт проходить через верхние дыхательные пути.",
  103: "Пауза в дыхании, во время которой временно пропадает дыхательное усилие.",
  104: "Во время этого эпизода поток воздуха при дыхании заметно снизился.",
  101: "Во время вдоха воздуху труднее проходить через верхние дыхательные пути.",
  100: "Эта отметка может указывать на храп во время дыхания.",
  151: "В эту минуту аппарат мог отметить начало значительной утечки воздуха.",
  150: "В эту минуту аппарат мог отметить окончание значительной утечки воздуха.",
};
const order = [102, 103, 104, 101, 100, 151, 150];

function groupBy(events: DeviceEvent[], key: "code" | "minute") {
  const groups = new Map<number, DeviceEvent[]>();
  for (const event of events) {
    const group = groups.get(event[key]);
    if (group) group.push(event);
    else groups.set(event[key], [event]);
  }
  return groups;
}

function displayName(event: DeviceEvent) {
  if (!event.label) return `Неизвестное событие · код ${event.code}`;
  if (event.meaning_evidence.includes("H")) {
    if (event.code === 100) return "Возможный храп";
    if (event.code === 151) return "Возможное начало большой утечки";
    if (event.code === 150) return "Возможный конец большой утечки";
  }
  return event.label;
}

export class EventTimeline {
  readonly element = document.createElement("section");
  private lanes: { minute: number; button: HTMLButtonElement }[][] = [];

  constructor(
    private readout: (text: string, x: number, y: number) => void,
    private dismiss: () => void,
  ) {
    this.element.className = "event-timeline";
    this.element.setAttribute("aria-label", "Лента событий");
    this.element.hidden = true;
  }

  show(events: DeviceEvent[], formatTime: (minute: number) => string) {
    this.clear();
    this.element.hidden = false;
    const heading = document.createElement("h2");
    heading.textContent = "Лента событий";
    const note = document.createElement("p");
    note.textContent = events.length
      ? "Выберите название, чтобы узнать его смысл, или отметку, чтобы посмотреть время."
      : "За этот сеанс аппарат не записал событий.";
    this.element.append(heading, note);

    const groups = groupBy(events, "code");
    const codes = [
      ...new Set([...order, ...[...groups.keys()].sort((a, b) => a - b)]),
    ];
    for (const code of codes) {
      const records = groups.get(code);
      if (!records) continue;
      const first = records[0];
      const name = displayName(first);
      const details = document.createElement("details");
      const summary = document.createElement("summary");
      const count = document.createElement("span");
      count.className = "event-count";
      count.textContent = String(records.length);
      summary.append(name, " ", count);
      const description = document.createElement("p");
      description.textContent =
        explanations[code] ||
        "Аппарат записал событие. Значение этого кода пока неизвестно.";
      details.append(summary, description);
      const track = document.createElement("div");
      track.className = "event-track";
      // Группируем только совпадающие тип и минуту. Счётчик сохраняет все повторы.
      const minutes = groupBy(records, "minute");
      const lane = [...minutes]
        .sort(([a], [b]) => a - b)
        .map(([minute, same]) => {
          const button = document.createElement("button");
          button.type = "button";
          button.className = "event-mark";
          const time = same[0].device_time?.slice(11, 16) || formatTime(minute);
          const text =
            `${time} · ${name}` + (same.length > 1 ? ` × ${same.length}` : "");
          button.setAttribute("aria-label", text);
          if (same.length > 1) button.classList.add("repeated");
          button.onpointermove = (event) => {
            if (event.pointerType !== "touch")
              this.readout(text, event.clientX, event.clientY);
          };
          const showAtMark = () => {
            const rect = button.getBoundingClientRect();
            this.readout(
              text,
              rect.left + rect.width / 2,
              rect.top + rect.height / 2,
            );
          };
          button.onclick = button.onfocus = showAtMark;
          button.onpointerleave = button.onblur = this.dismiss;
          track.append(button);
          return { minute, button };
        });
      this.lanes.push(lane);
      this.element.append(details, track);
    }
  }

  layout(pixel: (minute: number) => number, visible: (x: number) => boolean) {
    for (const lane of this.lanes) {
      const xs = lane.map((mark) => pixel(mark.minute));
      lane.forEach(({ button }, i) => {
        const x = xs[i];
        button.hidden = !visible(x);
        if (button.hidden) return;
        // Области нажатия соседних минут встречаются на середине расстояния.
        const left = Math.min(18, (x - (xs[i - 1] ?? -Infinity)) / 2);
        const right = Math.min(18, ((xs[i + 1] ?? Infinity) - x) / 2);
        button.style.left = x - left + "px";
        button.style.width = left + right + "px";
        button.style.setProperty("--tick-x", left + "px");
      });
    }
  }

  clear() {
    this.lanes = [];
    this.element.replaceChildren();
    this.element.hidden = true;
  }
}
