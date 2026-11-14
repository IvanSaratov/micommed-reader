import "./style.css";
import { Reader } from "./client";
import { SessionScreen } from "./sessions";
import type { Index } from "./types";

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
let reader: Reader | undefined;
let pendingFiles: File[] = [];
let session = 0;
const sleep = new SessionScreen();

function error(message: string) {
  text("error", message);
  show("error", true);
}
function close() {
  // Смена выгрузки отменяет старые запросы и удаляет все показанные значения.
  session++;
  sleep.clear();
  reader?.close();
  reader = undefined;
  pendingFiles = [];
  input("file-date").value = "";
  text("date-explanation", "");
  for (const id of [
    "workspace",
    "close-session",
    "loading",
    "date-request",
    "error",
  ])
    show(id, false);
  show("empty", true);
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
  reader = new Reader(progress);
  try {
    const result = await reader.call<Index>("open", {
      files: pendingFiles,
      date,
    });
    if (session !== generation) return;
    pendingFiles = [];
    show("workspace", true);
    show("close-session", true);
    await sleep.load(reader, result.reasons);
  } catch (e) {
    if (session !== generation) return;
    reader?.close();
    reader = undefined;
    pendingFiles = [];
    sleep.clear();
    show("workspace", false);
    show("close-session", false);
    error((e as Error).message);
    show("empty", true);
  } finally {
    if (session === generation) show("loading", false);
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
button("close-session").onclick = close;
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
