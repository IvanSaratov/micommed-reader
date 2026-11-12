import "./style.css";
import { Reader } from "./client";
import { DayCharts } from "./charts";
import { SessionScreen } from "./sessions";
import type { Index, Day, Metric, Problem, Source } from "./types";

const el = <T extends HTMLElement = HTMLElement>(id: string) =>
  document.getElementById(id) as T;
const show = (id: string, visible: boolean) => {
  el(id).hidden = !visible;
};
const text = (id: string, value: string) => {
  el(id).textContent = value;
};
const input = (id: string) => el<HTMLInputElement>(id);
const button = (id: string) => el<HTMLButtonElement>(id);
const number = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 });
let reader: Reader | undefined;
let index: Index | undefined;
let day: Day | undefined;
let pendingFiles: File[] = [];
let month = "";
let selectedDate = "";
let session = 0;
let dayRequest = 0;
let periodRequest = 0;
let exportRange = { from: "", through: "" };
let periodRange = { from: "", through: "" };
let charts: DayCharts | undefined;
const sleep = new SessionScreen();
let initialDay = "";

function error(message: string) {
  text("error", message);
  show("error", true);
}
function close() {
  session++;
  sleep.clear();
  initialDay = "";
  el<HTMLDetailsElement>("technical").open = false;
  reader?.close();
  reader = undefined;
  index = undefined;
  day = undefined;
  pendingFiles = [];
  month = "";
  selectedDate = "";
  text("month-title", "Нет дат");
  el("calendar").replaceChildren();
  for (const id of ["month-prev", "month-next", "day-prev", "day-next"])
    button(id).disabled = true;
  dayRequest++;
  periodRequest++;
  charts?.clear();
  // Сброс удаляет и скрытые строки DOM, чтобы следующий набор не показывал старый источник.
  for (const id of [
    "day-stats",
    "event-counts",
    "day-issues",
    "import-issues",
    "day-sources",
    "table-head",
    "table-body",
    "version",
    "period-stats",
    "source-name",
    "day-title",
    "day-time",
    "date-explanation",
    "interpretations",
    "table-evidence",
  ])
    el(id).replaceChildren();
  for (const id of ["from", "through", "file-date"]) input(id).value = "";
  for (const id of ["source-details", "interpretation-details"]) {
    show(id, false);
    el<HTMLDetailsElement>(id).open = false;
  }
  show("workspace", false);
  show("loading", false);
  show("date-request", false);
  show("error", false);
  show("empty", true);
  el<HTMLDialogElement>("export-dialog").close();
}
function progress(done: number, total: number) {
  const bar = el<HTMLProgressElement>("progress");
  bar.max = total || 1;
  bar.value = done;
  text("progress-label", "Прочитано файлов: " + done + " из " + total);
}
function pick(files: File[]) {
  if (!files.length) return;
  close();
  pendingFiles = [...files].sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  );
  if (
    files.length === 1 &&
    !/\.zip$/i.test(files[0].name) &&
    !/^\d{8}$/.test(files[0].name)
  ) {
    text(
      "date-explanation",
      "У файла «" +
        files[0].name +
        "» нет даты в имени. Укажите дату отчётного дня, который начинается в 09:00 аппарата.",
    );
    input("file-date").value = "";
    show("empty", false);
    show("date-request", true);
    input("file-date").focus();
  } else void openFiles("");
}
async function openFiles(date: string) {
  const generation = session;
  show("date-request", false);
  show("empty", false);
  show("error", false);
  show("loading", true);
  el<HTMLProgressElement>("progress").removeAttribute("value");
  text("progress-label", "Подготовка к чтению…");
  const name =
    pendingFiles.length === 1
      ? pendingFiles[0].name
      : pendingFiles.length + " дневных файлов";
  reader = new Reader(progress);
  try {
    const result = await reader.call<Index>("open", {
      files: pendingFiles,
      date,
    });
    if (session !== generation) return;
    pendingFiles = [];
    index = result;
    text("source-name", name);
    renderIndex(result);
    show("workspace", true);
    await sleep.load(reader);
    if (session !== generation) return;
    const sources = result.sources.filter((s) => s.date);
    const conflictDates = new Set(
      result.diagnostics
        .filter((d) => d.code === "conflicting_versions")
        .map((d) => d.date),
    );
    const preferred = sources.filter(
      (s) => s.status === "ok" && !conflictDates.has(s.date),
    );
    const chosen = preferred.at(-1) || sources.at(-1);
    initialDay = chosen?.date || "";
    if (chosen) {
      if (el<HTMLDetailsElement>("technical").open)
        await selectDay(chosen.date);
    } else {
      text("day-title", "Записи требуют проверки");
      text("day-time", "Причины указаны в диагностике выгрузки.");
      el("day-stats").replaceChildren();
      el("event-counts").replaceChildren();
      show("day-details", false);
      show("version-wrap", false);
      show("charts-wrap", false);
      show("table-wrap", false);
      button("export-day").disabled = true;
      button("zoom-reset").disabled = true;
    }
  } catch (e) {
    if (session !== generation) return;
    reader?.close();
    reader = undefined;
    pendingFiles = [];
    error((e as Error).message);
    show("empty", true);
  } finally {
    if (session === generation) show("loading", false);
  }
}
function dates() {
  return [
    ...new Set(index?.sources.map((s) => s.date).filter(Boolean) || []),
  ].sort();
}
function dateLabel(date: string) {
  return new Intl.DateTimeFormat("ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(date + "T12:00:00Z"));
}
function metric(m: Metric) {
  return m.value === null ? "—" : number.format(m.value);
}
function stat(container: HTMLElement, title: string, m: Metric, unit = "") {
  const pair = document.createElement("div");
  const dt = document.createElement("dt");
  dt.textContent = title;
  const dd = document.createElement("dd");
  dd.textContent = metric(m);
  const note = document.createElement("small");
  note.textContent = m.value === null ? reason(m.reason || "") : unit;
  dd.append(note);
  pair.append(dt, dd);
  container.append(pair);
}
function reason(code: string) {
  return index?.reasons[code] || code;
}
function issues(id: string, problems: Problem[]) {
  const area = el(id);
  area.replaceChildren();
  // Сгруппированные предупреждения оставляют список пригодным для чтения.
  const grouped = new Map<string, number>();
  for (const p of problems) {
    let label =
      p.code === "missing_dates"
        ? "Нет записей: " + p.date + " — " + p.through
        : (p.date ? p.date + " · " : "") + reason(p.code);
    if (p.field) label += " · " + p.field;
    if (p.kind === "exclusion") label += " · исключено из сводки";
    if (p.detail) label += " · " + p.detail;
    if (p.kind === "error" && p.label) label += " · " + p.label;
    grouped.set(label, (grouped.get(label) || 0) + 1);
  }
  for (const [label, count] of grouped) {
    const p = document.createElement("p");
    p.textContent = label + (count > 1 ? " × " + count : "");
    area.append(p);
  }
  if (!problems.length) {
    const p = document.createElement("p");
    p.textContent = "Замечаний нет";
    area.append(p);
  }
}
function renderIndex(value: Index) {
  input("from").value = value.period.from;
  input("through").value = value.period.through;
  periodRange = { from: value.period.from, through: value.period.through };
  renderPeriod(value);
  const entries = dates();
  text(
    "coverage",
    entries.length +
      " отчётных дней · " +
      value.sources.length +
      " файлов. Отметка под датой обозначает замечания.",
  );
  if (entries.length) month = entries.at(-1)!.slice(0, 7);
  renderCalendar();
}
function renderPeriod(value: Index) {
  const dl = el("period-stats");
  dl.replaceChildren();
  const values = [
    ["Учтено дней", String(value.period.included_days)],
    ["Минуты терапии", metric(value.period.therapy_minutes)],
    ["Событий для AHI", String(value.period.counted_events)],
    ["AHI по событиям", metric(value.period.recalculated_ahi)],
  ];
  for (const [title, value] of values) {
    const dt = document.createElement("dt");
    dt.textContent = title;
    const dd = document.createElement("dd");
    dd.textContent = value;
    dl.append(dt, dd);
  }
  if (value.period.recalculated_ahi.value === null) {
    const p = document.createElement("p");
    p.textContent = reason(value.period.recalculated_ahi.reason || "");
    dl.append(p);
  }
  issues("import-issues", value.diagnostics);
}
function renderCalendar() {
  const calendar = el("calendar");
  calendar.replaceChildren();
  if (!month) return;
  const first = new Date(month + "-01T12:00:00Z");
  text(
    "month-title",
    new Intl.DateTimeFormat("ru-RU", {
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    }).format(first),
  );
  const offset = (first.getUTCDay() + 6) % 7;
  for (let i = 0; i < offset; i++)
    calendar.append(document.createElement("span"));
  const count = new Date(
    Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0),
  ).getUTCDate();
  const available = dates();
  for (let i = 1; i <= count; i++) {
    const date = month + "-" + String(i).padStart(2, "0");
    const b = document.createElement("button");
    b.textContent = String(i);
    b.disabled = !available.includes(date);
    b.className = b.disabled ? "" : "has-data";
    b.setAttribute("aria-label", dateLabel(date));
    b.setAttribute("aria-pressed", String(date === selectedDate));
    if (index?.diagnostics.some((d) => d.date === date))
      b.dataset.status = "issue";
    b.onclick = () => {
      void selectDay(date);
    };
    calendar.append(b);
  }
  button("month-prev").disabled = !available.some((d) => d.slice(0, 7) < month);
  button("month-next").disabled = !available.some((d) => d.slice(0, 7) > month);
  const pos = available.indexOf(selectedDate);
  button("day-prev").disabled = pos <= 0;
  button("day-next").disabled = pos < 0 || pos === available.length - 1;
}
async function selectDay(date: string, sha?: string) {
  if (!index || !reader) return;
  const token = ++dayRequest;
  const generation = session;
  // Заголовок и данные должны относиться к одной версии, даже пока воркер занят.
  day = undefined;
  charts?.clear();
  el("day-stats").replaceChildren();
  el("event-counts").replaceChildren();
  show("day-details", false);
  show("charts-wrap", false);
  show("table-wrap", false);
  show("interpretation-details", false);
  selectedDate = date;
  month = date.slice(0, 7);
  renderCalendar();
  const sources = index.sources.filter((s) => s.date === date);
  el("day-sources").replaceChildren();
  for (const source of sources) {
    const item = document.createElement("li");
    item.textContent =
      source.label +
      (source.sha256 ? " · " + source.sha256.slice(0, 8) : " · ошибка чтения");
    el("day-sources").append(item);
  }
  show("source-details", true);
  const versions = [
    ...new Map(
      sources.filter((s) => s.status === "ok").map((s) => [s.sha256!, s]),
    ).values(),
  ];
  const select = el<HTMLSelectElement>("version");
  select.replaceChildren();
  versions.forEach((s) => {
    const o = document.createElement("option");
    o.value = s.sha256!;
    o.textContent = s.label + " · " + s.sha256!.slice(0, 8);
    select.append(o);
  });
  const chosen: Source | undefined =
    versions.find((s) => s.sha256 === sha) || versions[0];
  if (chosen) select.value = chosen.sha256!;
  show("version-wrap", versions.length > 1);
  text("day-title", dateLabel(date));
  text(
    "day-time",
    chosen ? "Открываем запись…" : "Данные этого дня повреждены",
  );
  button("export-day").disabled = false;
  button("zoom-reset").disabled = !chosen;
  if (!chosen) {
    day = undefined;
    charts?.clear();
    el("day-stats").replaceChildren();
    el("event-counts").replaceChildren();
    issues(
      "day-issues",
      index.diagnostics.filter((d) => d.date === date),
    );
    show("day-details", true);
    (el("day-details") as HTMLDetailsElement).open = true;
    show("charts-wrap", false);
    show("table-wrap", false);
    return;
  }
  try {
    const result = await reader.call<Day>("day", { date, sha: chosen.sha256 });
    if (token !== dayRequest || generation !== session) return;
    day = result;
    const meanings = el("interpretations");
    meanings.replaceChildren();
    for (const [id, field] of Object.entries(result.fields)) {
      const item = document.createElement("li");
      item.textContent = `${field.Label} (${id}) · ${field.MeaningEvidence}`;
      meanings.append(item);
    }
    show("interpretation-details", true);
    text(
      "day-time",
      result.start_device_time.replace("T", " · ") +
        " — " +
        result.end_device_time.replace("T", " · "),
    );
    const stats = el("day-stats");
    stats.replaceChildren();
    stat(stats, "Время терапии", result.therapy_minutes, "минут");
    stat(stats, "AHI аппарата", result.stored_ahi, "событий в час");
    stat(stats, "AHI по событиям", result.recalculated_ahi, "событий в час");
    const counts = el("event-counts");
    counts.replaceChildren();
    for (const [code, count] of Object.entries(result.event_counts)) {
      const e = result.events.find((e) => String(e.code) === code);
      const item = document.createElement("span");
      item.textContent =
        (e?.label || "Код " + code) + ` (${e?.meaning_evidence || "U"}) `;
      const value = document.createElement("strong");
      value.textContent = String(count);
      item.append(value);
      counts.append(item);
    }
    issues("day-issues", result.diagnostics);
    show("day-details", result.diagnostics.length > 0);
    (el("day-details") as HTMLDetailsElement).open = false;
    view("chart");
    charts ||= new DayCharts(el("charts"));
    charts.show(result);
    renderTable();
  } catch (e) {
    if (generation === session && token === dayRequest)
      error((e as Error).message);
  }
}
function view(kind: "chart" | "table") {
  show("charts-wrap", kind === "chart" && !!day);
  show("table-wrap", kind === "table" && !!day);
  button("chart-view").setAttribute("aria-pressed", String(kind === "chart"));
  button("table-view").setAttribute("aria-pressed", String(kind === "table"));
  if (kind === "chart") charts?.refresh();
}
function renderTable() {
  if (!day) return;
  const selected = el<HTMLSelectElement>("table-series").value;
  const field = day.fields[selected === "pressure" ? "pressure_1" : selected];
  text(
    "table-evidence",
    field
      ? `${field.Label} · ${field.MeaningEvidence}`
      : "Статус трактовки указан отдельно для каждого кода.",
  );
  let headings = ["Индекс", "Минута", "Время аппарата", "Исходное значение"];
  let rows: (string | number)[][];
  if (selected === "pressure") {
    headings = [...headings.slice(0, 3), "Ряд 1 · отсчёт", "Ряд 2 · отсчёт"];
    rows = day.pressure.map((p) => [
      p.index,
      p.minute,
      p.device_time?.replace("T", " ") || "—",
      p.first_raw,
      p.second_raw,
    ]);
  } else if (selected === "events") {
    headings = [...headings.slice(0, 3), "Код", "Название", "Трактовка"];
    rows = day.events.map((e) => [
      e.index,
      e.minute,
      e.device_time?.replace("T", " ") || "—",
      e.code,
      e.label || "Неизвестный код",
      e.meaning_evidence || "U",
    ]);
  } else
    rows = (day.channels.find((c) => c.field === selected)?.points || []).map(
      (p) => [
        p.index,
        p.minute,
        p.device_time?.replace("T", " ") || "—",
        p.raw,
      ],
    );
  const head = el("table-head");
  head.replaceChildren();
  const tr = document.createElement("tr");
  headings.forEach((h) => {
    const th = document.createElement("th");
    th.scope = "col";
    th.textContent = h;
    tr.append(th);
  });
  head.append(tr);
  const body = el("table-body");
  body.replaceChildren();
  rows.forEach((row) => {
    const tr = document.createElement("tr");
    row.forEach((value) => {
      const td = document.createElement("td");
      td.textContent = String(value);
      tr.append(td);
    });
    body.append(tr);
  });
}
function changeMonth(direction: number) {
  const d = new Date(month + "-01T12:00:00Z");
  d.setUTCMonth(d.getUTCMonth() + direction);
  month = d.toISOString().slice(0, 7);
  renderCalendar();
}
function exportDialog(range: { from: string; through: string }) {
  exportRange = range;
  text(
    "export-title",
    range.from === range.through ? "Скачать день" : "Скачать период",
  );
  el<HTMLDialogElement>("export-dialog").showModal();
}
async function download(format: "json" | "csv") {
  if (!reader) return;
  const generation = session;
  button("download-json").disabled = true;
  button("download-csv").disabled = true;
  try {
    const blob = await reader.call<Blob>("export", { ...exportRange, format });
    if (generation !== session) return;
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download =
      "healdata-" +
      exportRange.from +
      (exportRange.from !== exportRange.through
        ? "-" + exportRange.through
        : "") +
      (format === "json" ? ".json" : ".zip");
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    el<HTMLDialogElement>("export-dialog").close();
  } catch (e) {
    if (generation === session) {
      el<HTMLDialogElement>("export-dialog").close();
      error((e as Error).message);
    }
  } finally {
    button("download-json").disabled = false;
    button("download-csv").disabled = false;
  }
}
for (const id of ["open", "choose"])
  button(id).onclick = () => input("files").click();
input("files").onchange = () => {
  pick(Array.from(input("files").files || []));
  input("files").value = "";
};
el("date-form").onsubmit = (e) => {
  e.preventDefault();
  void openFiles(input("file-date").value);
};
button("date-cancel").onclick = close;
button("cancel").onclick = close;
button("close").onclick = close;
button("close-session").onclick = close;
el<HTMLDetailsElement>("technical").ontoggle = () => {
  if (!el<HTMLDetailsElement>("technical").open) return;
  if (!selectedDate && initialDay) void selectDay(initialDay);
  else charts?.refresh();
};
button("show-issues").onclick = () => {
  el<HTMLDetailsElement>("technical").open = true;
  el<HTMLDetailsElement>("import-details").open = true;
  el("technical").scrollIntoView({ block: "start" });
};
button("month-prev").onclick = () => changeMonth(-1);
button("month-next").onclick = () => changeMonth(1);
button("day-prev").onclick = () => {
  const d = dates();
  void selectDay(d[d.indexOf(selectedDate) - 1]);
};
button("day-next").onclick = () => {
  const d = dates();
  void selectDay(d[d.indexOf(selectedDate) + 1]);
};
el<HTMLSelectElement>("version").onchange = () => {
  void selectDay(selectedDate, el<HTMLSelectElement>("version").value);
};
button("chart-view").onclick = () => view("chart");
button("table-view").onclick = () => view("table");
el("table-series").onchange = renderTable;
button("zoom-reset").onclick = () => charts?.reset();
button("export-day").onclick = () =>
  exportDialog({ from: selectedDate, through: selectedDate });
button("export-period").onclick = () => exportDialog(periodRange);
button("download-json").onclick = () => {
  void download("json");
};
button("download-csv").onclick = () => {
  void download("csv");
};
el("period-form").onsubmit = async (e) => {
  e.preventDefault();
  if (!reader) return;
  const token = ++periodRequest;
  const generation = session;
  const range = { from: input("from").value, through: input("through").value };
  try {
    const result = await reader.call<Index>("period", range);
    if (generation !== session || token !== periodRequest) return;
    periodRange = range;
    renderPeriod(result);
    show("error", false);
  } catch (e) {
    if (generation === session && token === periodRequest)
      error((e as Error).message);
  }
};
document.addEventListener("dragover", (e) => {
  if (e.dataTransfer?.types.includes("Files")) {
    e.preventDefault();
    document.body.classList.add("dragging");
  }
});
document.addEventListener("dragleave", (e) => {
  if (!e.relatedTarget) document.body.classList.remove("dragging");
});
document.addEventListener("drop", (e) => {
  e.preventDefault();
  document.body.classList.remove("dragging");
  pick(Array.from(e.dataTransfer?.files || []));
});
