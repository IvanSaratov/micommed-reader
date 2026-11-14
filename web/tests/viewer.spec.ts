import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve, join } from "node:path";
import { zipSync } from "fflate";

// Только искусственные записи из публичных fixtures.
const root = resolve(import.meta.dirname, "../..");
const fixture = (name: string) =>
  readFileSync(join(root, "research/fixtures", name + ".bin"));
const file = (name: string, buffer: Buffer) => ({
  name,
  mimeType: "application/octet-stream",
  buffer,
});
const archive = (entries: Record<string, Uint8Array>) =>
  Buffer.from(zipSync(entries));
async function open(page: Page, name: string, bytes: Buffer, date?: string) {
  await page.locator("#files").setInputFiles(file(name, bytes));
  if (date) {
    await page.locator("#file-date").fill(date);
    await page
      .getByRole("button", { name: "Открыть запись", exact: true })
      .click();
  }
  await expect(page.locator("#workspace")).toBeVisible();
  await expect(page.locator("#loading")).toBeHidden();
}
test.beforeEach(async ({ page }) => {
  await page.goto("/");
});

test("renamed file opens sessions without a technical panel and clears on close", async ({
  page,
}) => {
  const failures: string[] = [];
  page.on("pageerror", (e) => failures.push(e.message));
  await open(
    page,
    "night.bin",
    sessionFixture(25, [840, 845, 850, 855]),
    "2001-02-25",
  );
  await expect(page.locator("#sleep-time")).toHaveText("23:00 — 23:15");
  await expect(page.locator("#technical")).toHaveCount(0);
  await expect(page.locator("#session-charts canvas")).toBeVisible();
  await page.locator("#close-session").focus();
  await page.keyboard.press("Enter");
  await expect(page.locator("#empty")).toBeVisible();
  await expect(page.locator("#sleep-stats")).toBeEmpty();
  expect(failures).toEqual([]);
});

test("ZIP duplicates do not add sessions and open data causes no network requests", async ({
  page,
  context,
}) => {
  await open(page, "20010203", fixture("clock"));
  const count = await page.locator("#sleep-select option").count();
  expect(count).toBeGreaterThan(0);
  await open(
    page,
    "records.zip",
    archive({
      "20010203": fixture("clock"),
      "copy/20010203": fixture("clock"),
    }),
  );
  await expect(page.locator("#sleep-select option")).toHaveCount(count);
  const requests: string[] = [];
  context.on("request", (r) => requests.push(r.url()));
  await page.locator("#session-prev").click();
  await page.locator("#session-next").click();
  await page.locator("#session-reset").click();
  expect(requests).toEqual([]);
});

test("conflicting and damaged records have a short explanation without details", async ({
  page,
}) => {
  await open(
    page,
    "conflict.zip",
    archive({
      "a/20010203": fixture("clock"),
      "b/20010203": fixture("minimum"),
      "20010204": fixture("next-day"),
    }),
  );
  await expect(page.locator("#session-warning")).toContainText("разные версии");
  await expect(page.locator("#sleep-result")).toBeVisible();
  await open(page, "broken.zip", archive({ "20010203": fixture("truncated") }));
  await expect(page.locator("#sleep-empty")).toBeVisible();
  await expect(page.locator("#session-warning")).not.toBeEmpty();
  await expect(page.locator("#sleep-result")).toBeHidden();
});

test("multiple files, drop, cancel and replacement, narrow screen", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .locator("#files")
    .setInputFiles([
      file("20010203", fixture("clock")),
      file("20010204", fixture("next-day")),
    ]);
  await expect(page.locator("#loading")).toBeHidden();
  await expect(page.locator("#sleep-select")).not.toBeEmpty();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const payload = Array.from(fixture("clock"));
  await page.evaluate((bytes) => {
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(new File([new Uint8Array(bytes)], "20010203"));
    document.dispatchEvent(new DragEvent("drop", { dataTransfer }));
  }, payload);
  await expect(page.locator("#loading")).toBeHidden();
  await expect(page.locator("#sleep-select")).not.toBeEmpty();
  await page.locator("#close-session").click();
  await expect(page.locator("#empty")).toBeVisible();
  const big = archive(
    Object.fromEntries(
      Array.from({ length: 4096 }, (_, i) => [
        `${i}/20010203`,
        fixture("clock"),
      ]),
    ),
  );
  await page.locator("#files").setInputFiles(file("large.zip", big));
  await page.locator("#cancel").click();
  await expect(page.locator("#empty")).toBeVisible();
  await open(page, "20010225", sessionFixture(25, [840, 850]));
  await expect(page.locator("#sleep-time")).toHaveText("23:00 — 23:10");
  await expect(page.locator("#error")).toBeHidden();
});

test("damaged ZIP and entry limit produce recoverable errors", async ({
  page,
}) => {
  await page
    .locator("#files")
    .setInputFiles(file("bad.zip", Buffer.from("not a zip")));
  await expect(page.locator("#error")).toBeVisible();
  const tooMany = archive(
    Object.fromEntries(
      Array.from({ length: 4097 }, (_, i) => [`empty-${i}`, new Uint8Array()]),
    ),
  );
  await page.locator("#files").setInputFiles(file("over-limit.zip", tooMany));
  await expect(page.locator("#error")).toContainText("4096");
  await open(page, "20010203", fixture("clock"));
  await expect(page.locator("#error")).toBeHidden();
});

test("relative deployment path and readable startup failure", async ({
  page,
  context,
}) => {
  // Отдаём ту же production-сборку под подпутём, как на GitHub Pages.
  await context.route("**/viewer/**", async (route) => {
    const path =
      new URL(route.request().url()).pathname.slice("/viewer/".length) ||
      "index.html";
    const types: Record<string, string> = {
      html: "text/html",
      js: "text/javascript",
      css: "text/css",
      wasm: "application/wasm",
      woff2: "font/woff2",
    };
    await route.fulfill({
      body: readFileSync(join(root, "web/dist", path)),
      contentType: types[path.split(".").at(-1)!] || "application/octet-stream",
    });
  });
  await page.goto("/viewer/");
  await open(page, "20010203", fixture("clock"));
  await expect(page.locator("#sleep-result")).toBeVisible();
  await context.route("**/wasm/healdata.wasm", (route) =>
    route.fulfill({ status: 503, body: "unavailable" }),
  );
  await page
    .locator("#files")
    .setInputFiles(file("20010203", fixture("clock")));
  await expect(page.locator("#error")).toContainText(
    "Не удалось загрузить модуль",
  );
  await expect(page.locator("#loading")).toBeHidden();
});

test("maximum-size container keeps page responsive and preserves duplicates", async ({
  page,
}, testInfo) => {
  const bytes = archive(
    Object.fromEntries(
      Array.from({ length: 4096 }, (_, i) => [
        `${i}/20010203`,
        fixture("clock"),
      ]),
    ),
  );
  await page.evaluate(() => {
    document.body.dataset.ticks = "0";
    const tick = () => {
      document.body.dataset.ticks = String(
        Number(document.body.dataset.ticks) + 1,
      );
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const started = Date.now();
  await open(page, "large.zip", bytes);
  // В искусственной записи два участка: 895–900 и 1439 минут.
  await expect(page.locator("#sleep-select option")).toHaveCount(2);
  await expect(page.locator("#sleep-result")).toBeVisible();
  expect(
    Number(await page.locator("body").getAttribute("data-ticks")),
  ).toBeGreaterThan(3);
  await testInfo.attach("large-import.json", {
    body: JSON.stringify({
      files: 4096,
      rawBytes: 4096 * 9312,
      elapsedMs: Date.now() - started,
    }),
    contentType: "application/json",
  });
});

test("replacement with invalid dates clears the previous session", async ({
  page,
}) => {
  await open(page, "20010225", sessionFixture(25, [840, 850]));
  await expect(page.locator("#sleep-stats")).not.toBeEmpty();
  await open(page, "20010230", fixture("clock"));
  await expect(page.locator("#sleep-empty")).toBeVisible();
  await expect(page.locator("#sleep-stats")).toBeEmpty();
  await expect(page.locator("#sleep-select")).toBeEmpty();
});

function sessionFixture(day: number, minutes: number[]) {
  const raw = Buffer.from(fixture("clock"));
  const origin = Date.UTC(2001, 1, day, 9);
  const stamp = (offset: number, minute: number) => {
    const date = new Date(origin + minute * 60000);
    raw.writeUInt16LE(date.getUTCFullYear(), offset);
    [
      date.getUTCMonth() + 1,
      date.getUTCDate(),
      date.getUTCHours(),
      date.getUTCMinutes(),
      0,
    ].forEach((n, i) => {
      raw[offset + 2 + i] = n;
    });
  };
  stamp(0xb30, minutes[0]);
  stamp(0xb38, minutes.at(-1)!);
  raw.writeUInt16LE(minutes.length, 0);
  minutes.forEach((m, i) => {
    raw.writeUInt16LE(m, 0x648 + i * 2);
    raw.writeUInt16LE(70 + i, 8 + i * 2);
    raw.writeUInt16LE(100 + i, 0x328 + i * 2);
  });
  for (const [count, times] of [
    [0xf18, 0x104c],
    [0x12f4, 0x153c],
    [0x1780, 0x18a4],
    [0x1ae8, 0x1c0c],
    [0x1e50, 0x1f74],
  ]) {
    raw.writeUInt16LE(minutes.length, count);
    minutes.forEach((m, i) => raw.writeUInt16LE(m, times + i * 2));
  }
  raw.writeUInt16LE(0, 0x968);
  for (const offset of [0xe04, 0xe06, 0xe08, 0xe0a])
    raw.writeUInt16LE(0, offset);
  raw.writeFloatLE(30, 0xddc);
  return raw;
}

test("calendar selects whole sessions across months and offers each session on a shared date", async ({
  page,
}) => {
  const archiveBytes = archive({
    "20010228": sessionFixture(28, [895, 900, 905, 1200, 1220]),
  });
  await open(page, "calendar.zip", archiveBytes);
  const calendar = page.getByRole("navigation", { name: "Календарь записей" });
  const day = (key: string) => calendar.locator(`button[data-date="${key}"]`);
  await expect(day("2001-03-01")).toHaveAttribute("aria-pressed", "true");
  await expect(day("2001-03-02")).toBeDisabled();
  await expect(page.locator("#sleep-time")).toHaveText("05:00 — 05:20");
  await expect(
    page.getByLabel("Сеанс за выбранный день").locator("option"),
  ).toHaveCount(2);
  await page.getByLabel("Предыдущий месяц с записями").click();
  await day("2001-02-28").focus();
  await page.keyboard.press("Enter");
  await expect(day("2001-02-28")).toBeFocused();
  await expect(page.locator("#sleep-time")).toHaveText("23:55 — 00:05");
  await expect(page.locator("#sleep-stats")).toContainText("10 мин");
  await expect(page.locator("#day-sessions")).toBeHidden();
  await page.getByLabel("Следующий месяц с записями").click();
  await day("2001-03-01").click();
  const picker = page.getByLabel("Сеанс за выбранный день");
  await expect(picker).toHaveValue("0");
  await picker.selectOption("1");
  await expect(page.locator("#sleep-stats")).toContainText("20 мин");
  await expect(page.locator("#sleep-time")).toHaveText("05:00 — 05:20");
  await expect(day("2001-03-01")).toHaveAttribute("aria-pressed", "true");
  await page.locator("#close-session").click();
  await expect(calendar.locator("[data-date]")).toHaveCount(0);
});

test("simple sleep session spans 09:00 and separates the next evening", async ({
  page,
}) => {
  const night = [...Array.from({ length: 20 }, (_, i) => 840 + i * 30), 1439];
  const bytes = archive({
    "20010225": sessionFixture(25, night),
    "20010226": sessionFixture(26, [0, 20, 40, 840, 870]),
  });
  await open(page, "nights.zip", bytes);
  await expect(page.locator("#technical")).toHaveCount(0);
  await expect(page.locator("#sleep-select option")).toHaveCount(2);
  await page.locator("#sleep-select").selectOption("0");
  await expect(page.locator("#sleep-time")).toHaveText("23:00 — 09:40");
  await expect(page.locator("#sleep-stats")).toContainText("10 ч 40 мин");
  await expect(page.locator("#sleep-stats dd").last()).toContainText("—");
  await expect(page.locator("#session-charts canvas")).toBeVisible();
  await expect(
    page.getByText("За этот сеанс аппарат не записал событий."),
  ).toBeVisible();
  await expect(page.locator(".event-track")).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page
        .locator("#session-charts canvas")
        .evaluate((canvas: HTMLCanvasElement) => canvas.clientWidth),
    )
    .toBe(358);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.locator("#close-session").click();
  await expect(page.locator("#empty")).toBeVisible();
  await expect(page.locator("#sleep-select")).toBeEmpty();
});

test("session hover stays on the active metric between samples and clears on zoom", async ({
  page,
}) => {
  const raw = sessionFixture(25, [840, 845, 850, 855]);
  raw.writeUInt16LE(70, 0x328);
  [12, 14, 16, 18].forEach((value, i) => {
    raw[0xf20 + i] = value;
  });
  await open(page, "20010225", raw);
  const charts = page.locator("#session-charts");
  const reading = charts.getByRole("tooltip", { includeHidden: true });
  async function hover(axis: number, minute: number) {
    await charts.evaluate(
      (el, row) =>
        window.scrollTo(
          0,
          el.getBoundingClientRect().top + scrollY + row * 360 - 40,
        ),
      axis,
    );
    // scroll обрабатывается перед наведением, как при завершённой прокрутке пользователем.
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    const box = (await charts.boundingBox())!;
    const point = {
      x: box.x + 48 + ((box.width - 60) * minute) / 15,
      y: box.y + axis * 360 + 112 + 108,
    };
    await page.mouse.move(point.x, point.y);
    return point;
  }
  async function expectReading(time: string, values: string[], unit: string) {
    await expect(reading.locator("time")).toHaveText(time);
    await expect(reading.locator("strong")).toHaveText(values);
    await expect(reading.locator("small")).toHaveText([unit]);
  }
  await hover(0, 1);
  await expectReading("23:00", ["7"], "см вод. ст.");
  await expect(reading).toBeVisible();
  const position = (await reading.boundingBox())!;
  await hover(0, 2);
  await expect(reading).toBeVisible();
  await expectReading("23:00", ["7"], "см вод. ст.");
  await hover(0, 4);
  await expectReading("23:05", ["7,1", "10,1"], "см вод. ст.");
  expect((await reading.boundingBox())!.x).toBeGreaterThan(position.x + 50);
  await hover(1, 4);
  await expectReading("23:05", ["14"], "л/мин");
  await page.keyboard.down("Shift");
  await page.mouse.wheel(0, -400);
  await page.keyboard.up("Shift");
  await expect(reading).toBeHidden();
  await page.locator("#session-reset").click();
  await hover(0, 4);
  await expectReading("23:05", ["7,1", "10,1"], "см вод. ст.");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      charts
        .locator("canvas")
        .evaluate((el: HTMLCanvasElement) => el.clientWidth),
    )
    .toBe(358);
  await hover(1, 4);
  await expectReading("23:05", ["14"], "л/мин");
  const bounds = (await reading.boundingBox())!;
  expect(bounds.height).toBeLessThan(70);
  expect(bounds.x).toBeGreaterThanOrEqual(8);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(382);
  const edge = await hover(1, 14);
  await expectReading("23:15", ["18"], "л/мин");
  await expect(reading).toBeVisible();
  const edgeBounds = (await reading.boundingBox())!;
  expect(edgeBounds.x + edgeBounds.width).toBeLessThan(edge.x);
  expect(edgeBounds.y + edgeBounds.height).toBeLessThanOrEqual(836);
  await page.locator("#close-session").click();
  await expect(reading).toBeHidden();
  await expect(reading).toBeEmpty();
});

test("ordinary wheel scrolls the page while the pointer is over a graph", async ({
  page,
}) => {
  const raw = sessionFixture(25, [840, 845, 850, 855]);
  await open(page, "20010225", raw);
  const charts = page.locator("#session-charts");
  await charts.evaluate((el) =>
    window.scrollTo(0, el.getBoundingClientRect().top + scrollY - 30),
  );
  const box = (await charts.boundingBox())!;
  await page.mouse.move(box.x + 150, box.y + 112 + 108);
  const before = await page.evaluate(() => scrollY);
  await page.mouse.wheel(0, 350);
  await expect
    .poll(() => page.evaluate(() => scrollY))
    .toBeGreaterThan(before + 200);
  await expect(
    charts.getByRole("tooltip", { includeHidden: true }),
  ).toBeHidden();
});

test("event lanes retain repeat counts, definitions and the shared zoom across midnight", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const raw = sessionFixture(25, [895, 900, 905, 910]);
  const events = [
    [895, 100],
    [900, 104],
    [900, 104],
    [900, 238],
    [910, 104],
  ];
  raw.writeUInt16LE(events.length, 0x968);
  events.forEach(([minute, code], i) => {
    raw[0x96a + i] = code;
    raw.writeUInt16LE(minute, 0xa00 + i * 2);
  });
  raw.writeUInt16LE(3, 0xe0a);
  await open(page, "20010225", raw);
  const timeline = page.getByRole("region", { name: "Лента событий" });
  const labels = timeline.locator("summary");
  await expect(labels).toHaveText([
    "Гипопноэ 3",
    "Возможный храп 1",
    "Неизвестное событие · код 238 1",
  ]);
  await expect(
    page
      .locator("#sleep-stats > div")
      .filter({ hasText: "Апноэ и гипопноэ" })
      .locator("dd"),
  ).toContainText("3");
  const definition = timeline.getByText(/Во время этого эпизода/);
  await labels.first().focus();
  await page.keyboard.press("Enter");
  await expect(definition).toBeVisible();
  await labels.first().click();
  await expect(definition).toBeHidden();
  const repeat = timeline.getByRole("button", { name: "00:00 · Гипопноэ × 2" });
  const other = timeline.getByRole("button", {
    name: "00:00 · Неизвестное событие · код 238",
  });
  const last = timeline.getByRole("button", { name: "00:10 · Гипопноэ" });
  await expect(last).toBeVisible();
  await repeat.click();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toHaveText("00:00 · Гипопноэ × 2");
  await other.hover();
  await expect(tooltip).toHaveText("00:00 · Неизвестное событие · код 238");
  const x = (await repeat.boundingBox())!.x;
  const chart = page.locator("#session-charts");
  await chart.evaluate((el) =>
    window.scrollTo(0, el.getBoundingClientRect().top + scrollY),
  );
  const box = (await chart.boundingBox())!;
  await page.mouse.move(box.x + 48, box.y + 220);
  await page.keyboard.down("Shift");
  await page.mouse.wheel(0, -450);
  await page.keyboard.up("Shift");
  await expect(last).toBeHidden();
  await expect
    .poll(async () => (await repeat.boundingBox())?.x ?? 0)
    .toBeGreaterThan(x + 1);
  const fifth = chart.locator("summary").filter({ hasText: "Пятый канал" });
  const fifthDescription = chart.getByText(/Дополнительный сигнал аппарата/);
  await expect(fifthDescription).toHaveCount(0);
  const zoomedX = (await repeat.boundingBox())!.x;
  await fifth.focus();
  await page.keyboard.press("Enter");
  await expect(fifthDescription).toBeVisible();
  await expect(last).toBeHidden();
  expect((await repeat.boundingBox())!.x).toBeCloseTo(zoomedX, 0);
  await fifth.click();
  await expect(fifthDescription).toHaveCount(0);
  await expect(last).toBeHidden();
  await page.locator("#session-reset").click();
  await expect(last).toBeVisible();
  await repeat.click();
  await expect(tooltip).toHaveText("00:00 · Гипопноэ × 2");
  await page.keyboard.press("Escape");
  await expect(tooltip).toBeHidden();
  await open(page, "20010225", sessionFixture(25, [840, 845]));
  await expect(
    timeline.getByText("За этот сеанс аппарат не записал событий."),
  ).toBeVisible();
  await expect(labels).toHaveCount(0);
});
