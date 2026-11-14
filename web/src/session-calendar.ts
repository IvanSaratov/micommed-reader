import type { SleepSession } from "./types";

const monthName = new Intl.DateTimeFormat("ru-RU", {
  month: "long",
  year: "numeric",
  timeZone: "UTC",
});
const dayName = new Intl.DateTimeFormat("ru-RU", {
  dateStyle: "full",
  timeZone: "UTC",
});
const dayKey = (date: Date) => date.toISOString().slice(0, 10);
const midnight = (day: string) => new Date(day + "T00:00:00Z");

export class SessionCalendar {
  private byDate = new Map<string, SleepSession[]>();
  private months: string[] = [];
  private month = "";
  private selected = "";
  private title = document.getElementById("calendar-month")!;
  private days = document.getElementById("calendar-days")!;
  private prev = document.getElementById("month-prev") as HTMLButtonElement;
  private next = document.getElementById("month-next") as HTMLButtonElement;

  constructor(onDay: (day: string, sessions: SleepSession[]) => void) {
    this.prev.onclick = () => this.browse(-1);
    this.next.onclick = () => this.browse(1);
    this.days.onclick = (event) => {
      const button = (event.target as HTMLElement).closest<HTMLButtonElement>(
        "button[data-date]",
      );
      const day = button?.dataset.date;
      if (day && this.byDate.has(day)) onDay(day, this.byDate.get(day)!);
    };
  }

  load(sessions: SleepSession[]) {
    this.clear();
    // Используем календарные даты аппарата. Ночь через полночь открывает один
    // и тот же сеанс по каждой дате, включая переход месяца или года.
    for (const session of sessions) {
      const date = midnight(session.start_device_time.slice(0, 10));
      const end = midnight(session.end_device_time.slice(0, 10));
      for (; date <= end; date.setUTCDate(date.getUTCDate() + 1)) {
        const key = dayKey(date);
        const list = this.byDate.get(key);
        if (list) list.push(session);
        else this.byDate.set(key, [session]);
      }
    }
    this.months = [
      ...new Set([...this.byDate.keys()].map((day) => day.slice(0, 7))),
    ].sort();
  }

  select(day: string) {
    this.selected = day;
    const month = day.slice(0, 7);
    if (month !== this.month) this.render(month);
    else {
      this.days
        .querySelector("[aria-pressed=true]")
        ?.setAttribute("aria-pressed", "false");
      this.days
        .querySelector(`[data-date="${day}"]`)
        ?.setAttribute("aria-pressed", "true");
    }
    return this.byDate.get(day) || [];
  }

  private browse(step: number) {
    const month = this.months[this.months.indexOf(this.month) + step];
    if (month) this.render(month);
  }

  private render(month: string) {
    this.month = month;
    const date = midnight(month + "-01");
    this.title.textContent = monthName.format(date).replace(/\s*г\.$/, "");
    const index = this.months.indexOf(month);
    this.prev.disabled = index <= 0;
    this.next.disabled = index >= this.months.length - 1;
    const firstColumn = ((date.getUTCDay() + 6) % 7) + 1;
    const fragment = document.createDocumentFragment();
    const monthIndex = date.getUTCMonth();
    while (date.getUTCMonth() === monthIndex) {
      const day = date.getUTCDate();
      const key = dayKey(date);
      const button = document.createElement("button");
      button.type = "button";
      button.dataset.date = key;
      button.textContent = String(day);
      button.setAttribute("aria-label", dayName.format(date));
      button.disabled = !this.byDate.has(key);
      if (!button.disabled)
        button.setAttribute("aria-pressed", String(key === this.selected));
      if (day === 1) button.style.gridColumnStart = String(firstColumn);
      fragment.append(button);
      date.setUTCDate(day + 1);
    }
    this.days.replaceChildren(fragment);
  }

  clear() {
    this.byDate.clear();
    this.months = [];
    this.selected = this.month = "";
    this.title.textContent = "";
    this.days.replaceChildren();
    this.prev.disabled = this.next.disabled = true;
  }
}
