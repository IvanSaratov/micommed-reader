import { test, expect, type Page } from "@playwright/test";
import {
  readFileSync,
  writeFileSync,
  mkdtempSync,
  readdirSync,
  rmSync,
} from "node:fs";
import { execFileSync, spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { zipSync, unzipSync } from "fflate";

// Только искусственные записи из публичных fixtures; личные выгрузки здесь не нужны.
const root = resolve(import.meta.dirname, "../..");
const fixture = (name: string) =>
  readFileSync(join(root, "research/fixtures", name + ".bin"));
let temp: string;
let cli: string;
test.beforeAll(() => {
  temp = mkdtempSync(join(tmpdir(), "healdata-browser-"));
  cli = join(temp, "healdata");
  execFileSync("go", ["build", "-o", cli, "./cmd/healdata"], { cwd: root });
});
test.afterAll(() => rmSync(temp, { recursive: true, force: true }));

const file = (name: string, buffer: Buffer) => ({
  name,
  mimeType: "application/octet-stream",
  buffer,
});
const archive = (entries: Record<string, Uint8Array>) =>
  Buffer.from(zipSync(entries));
async function open(
  page: Page,
  name: string,
  bytes: Buffer,
  date?: string,
  technical = true,
) {
  await page.locator("#files").setInputFiles(file(name, bytes));
  if (date) {
    await page.locator("#file-date").fill(date);
    await page
      .getByRole("button", { name: "Открыть запись", exact: true })
      .click();
  }
  await expect(page.locator("#workspace")).toBeVisible();
  await expect(page.locator("#loading")).toBeHidden();
  if (technical) await page.locator("#technical > summary").click();
}
async function download(
  page: Page,
  scope: "day" | "period",
  format: "json" | "csv",
) {
  await page.locator("#export-" + scope).click();
  const download = page.waitForEvent("download");
  await page.locator("#download-" + format).click();
  const path = await (await download).path();
  return readFileSync(path!);
}
function native(bytes: Buffer, from?: string, through?: string) {
  const source = join(temp, "input.zip");
  writeFileSync(source, bytes);
  const args = [
    "export",
    "json",
    ...(from ? ["--from", from, "--to", through!] : []),
    source,
  ];
  const result = spawnSync(cli, args, { encoding: "utf8" });
  expect([0, 3]).toContain(result.status);
  return { doc: JSON.parse(result.stdout), source };
}
test.beforeEach(async ({ page }) => {
  await page.goto("/");
});

test("single renamed file, clock, repeated minute, unknown event, keyboard and reset", async ({
  page,
}) => {
  const failures: string[] = [];
  page.on("pageerror", (e) => failures.push(e.message));
  await open(page, "clock.bin", fixture("clock"), "2001-02-03");
  await expect(page.locator("#day-stats")).toContainText("30");
  await expect(page.locator("#day-stats dd").nth(1)).toHaveText(
    "2событий в час",
  );
  await expect(page.locator("#day-stats dd").nth(2)).toHaveText(
    "2событий в час",
  );
  await expect(page.locator("#event-counts")).toContainText("238");
  await page.locator("#table-view").focus();
  await page.keyboard.press("Enter");
  await expect(page.locator("#table-body tr")).toHaveCount(5);
  const rows = page
    .locator("#table-body tr")
    .filter({ hasText: "2001-02-04 00:00:00" });
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText("103");
  await expect(rows.last()).toContainText("104");
  await page.locator("#table-series").selectOption("events");
  await expect(page.locator("#table-body tr")).toHaveCount(3);
  await expect(page.locator("#table-body tr").nth(1)).toContainText("238");
  await page.locator("#chart-view").click();
  await expect(page.locator("#charts canvas")).toBeVisible();
  await page.locator("#charts").scrollIntoViewIfNeeded();
  const box = (await page.locator("#charts").boundingBox())!;
  await page.mouse.move(
    box.x + 48 + ((box.width - 60) * 1000) / 1439,
    box.y + 90,
  );
  await expect(page.locator("#charts [role=tooltip]")).toBeHidden();
  await page.mouse.move(
    Math.ceil(box.x + 48 + ((box.width - 60) * 900) / 1439),
    box.y + 90,
  );
  await expect(
    page.getByText("Событие #1 · 238", { exact: false }),
  ).toBeVisible();
  await page.mouse.move(
    box.x + 48 + ((box.width - 60) * 1000) / 1439,
    box.y + 90,
  );
  await expect(page.locator("#charts [role=tooltip]")).toBeHidden();
  await page.locator("#zoom-reset").click();
  const doc = JSON.parse((await download(page, "day", "json")).toString());
  expect(doc.days[0].events.map((e: { code: number }) => e.code)).toEqual([
    102, 238, 101,
  ]);
  expect(failures).toEqual([]);
});

test("ZIP duplicates, inclusive period, exact CLI JSON/CSV parity and no data requests", async ({
  page,
  context,
}) => {
  const bytes = archive({
    "HealData/20010203": fixture("clock"),
    "copy/20010203": fixture("clock"),
    "HealData/20010204": fixture("next-day"),
  });
  const { doc, source } = native(bytes);
  await open(page, "records.zip", bytes);
  await expect(page.locator("#period-stats")).toContainText("31");
  expect(doc.period.recalculated_ahi.value).toBeCloseTo(60 / 31);
  const requests: string[] = [];
  context.on("request", (r) => {
    if (!r.url().startsWith("blob:")) requests.push(r.url());
  });
  const browser = JSON.parse(
    (await download(page, "period", "json")).toString(),
  );
  expect(browser).toEqual(doc);
  const tables = unzipSync(await download(page, "period", "csv"));
  const csvDir = join(temp, "csv-" + Date.now());
  const run = spawnSync(cli, ["export", "csv", "--out", csvDir, source], {
    encoding: "utf8",
  });
  expect([0, 3]).toContain(run.status);
  expect(Object.keys(tables).sort()).toEqual(readdirSync(csvDir).sort());
  for (const name of Object.keys(tables))
    expect(Buffer.from(tables[name])).toEqual(readFileSync(join(csvDir, name)));
  await page.locator("#from").fill("2001-02-03");
  await page.locator("#through").fill("2001-02-03");
  await page.getByRole("button", { name: "Рассчитать", exact: true }).click();
  await expect(page.locator("#period-stats")).toContainText("30");
  expect(
    JSON.parse((await download(page, "period", "json")).toString()),
  ).toEqual(native(bytes, "2001-02-03", "2001-02-03").doc);
  await page.locator("#from").fill("2001-03-01");
  await page.locator("#through").fill("2001-03-02");
  await page.getByRole("button", { name: "Рассчитать", exact: true }).click();
  await expect(page.locator("#period-stats dd").first()).toHaveText("0");
  const empty = JSON.parse((await download(page, "period", "json")).toString());
  expect(empty.days).toEqual([]);
  expect(empty.period.recalculated_ahi.value).toBeNull();
  expect(requests).toEqual([]);
});

test("conflict versions, unavailable stored metric and invalid record diagnostics", async ({
  page,
}) => {
  const bytes = archive({
    "a/20010203": fixture("clock"),
    "b/20010203": fixture("minimum"),
    "20010204": fixture("next-day"),
  });
  await open(page, "conflict.zip", bytes);
  await expect(page.locator("#period-stats dd").first()).toHaveText("1");
  await page.locator("#day-prev").click();
  await expect(page.locator("#version-wrap")).toBeVisible();
  await expect(page.locator("#version option")).toHaveCount(2);
  await expect(page.locator("#day-stats dd").first()).toHaveText("30минут");
  const first = await page.locator("#day-stats dd").first().textContent();
  const second = await page
    .locator("#version option")
    .nth(1)
    .getAttribute("value");
  await page.locator("#version").selectOption(second!);
  await expect(page.locator("#day-stats dd").first()).not.toHaveText(first!);
  expect(JSON.parse((await download(page, "day", "json")).toString())).toEqual(
    native(bytes, "2001-02-03", "2001-02-03").doc,
  );
  await open(page, "20010203", fixture("nan"));
  await expect(page.locator("#day-stats dd").nth(1)).toContainText("—");
  await expect(page.locator("#day-stats dd").nth(2)).toHaveText(
    "2событий в час",
  );
  await open(page, "broken.zip", archive({ "20010203": fixture("truncated") }));
  await expect(page.locator("#day-time")).toContainText("повреждены");
  await expect(page.locator("#day-issues")).not.toBeEmpty();
  await expect(page.locator("#charts-wrap")).toBeHidden();
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
  await page.locator("#technical > summary").click();
  await expect(page.locator("#period-stats")).toContainText("31");
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
  await page.locator("#technical > summary").click();
  await expect(page.locator("#period-stats")).toContainText("30");
  await page.locator("#close").click();
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
  await open(page, "20010203", fixture("minimum"));
  await expect(page.locator("#day-stats dd").first()).toHaveText("1минут");
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
  await expect(page.locator("#day-stats dd").first()).toHaveText("30минут");
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
  await expect(page.locator("#coverage")).toContainText("4096");
  await expect(page.locator("#period-stats dd").first()).toHaveText("1");
  await expect(page.locator("#day-stats dd").first()).toHaveText("30минут");
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

test("interpretation evidence remains visible in data views", async ({
  page,
}) => {
  await open(page, "20010203", fixture("clock"));
  await page.locator("#table-view").click();
  await expect(page.locator("#table-evidence")).toContainText(
    "Предполагаемое давление",
  );
  await expect(page.locator("#table-evidence")).toContainText("C+H");
  await page.locator("#table-series").selectOption("events");
  await expect(
    page.locator("#table-body tr").first().locator("td").last(),
  ).toHaveText("C");
  await expect(
    page.locator("#table-body tr").nth(1).locator("td").last(),
  ).toHaveText("U");
  await page.locator("#interpretation-details summary").click();
  await expect(page.locator("#interpretation-details")).toContainText(
    "H — гипотеза",
  );
});

test("replacement with invalid dates clears all previous source details", async ({
  page,
}) => {
  await open(page, "20010203", fixture("clock"));
  await page.locator("#source-details summary").click();
  await expect(page.locator("#day-sources")).toContainText("20010203");
  await open(page, "20010230", fixture("clock"));
  await expect(page.locator("#day-title")).toHaveText(
    "Записи требуют проверки",
  );
  await expect(page.locator("#source-details")).toBeHidden();
  await expect(page.locator("#day-sources")).toBeEmpty();
  await expect(page.locator("#table-body")).toBeEmpty();
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

test("simple sleep session spans 09:00 and separates the next evening", async ({
  page,
}) => {
  const night = [...Array.from({ length: 20 }, (_, i) => 840 + i * 30), 1439];
  const bytes = archive({
    "20010225": sessionFixture(25, night),
    "20010226": sessionFixture(26, [0, 20, 40, 840, 870]),
  });
  await open(page, "nights.zip", bytes, undefined, false);
  await expect(page.locator("#technical")).not.toHaveAttribute("open", "");
  await expect(page.locator("#source-name")).toBeHidden();
  await expect(page.locator("#sleep-select option")).toHaveCount(2);
  await page.locator("#sleep-select").selectOption("0");
  await expect(page.locator("#sleep-time")).toHaveText("23:00 — 09:40");
  await expect(page.locator("#sleep-stats")).toContainText("10 ч 40 мин");
  await expect(page.locator("#sleep-stats dd").last()).toContainText("—");
  await expect(page.locator("#session-charts canvas")).toBeVisible();
  await expect(page.locator("#session-charts")).toHaveCSS("height", "395px");
  await page.locator("#session-more").click();
  await expect(page.locator("#session-charts")).toHaveCSS("height", "1235px");
  await page.locator("#session-more").click();
  await page.locator("#technical > summary").click();
  await page.locator("#session-gap").fill("1");
  await page.getByRole("button", { name: "Применить", exact: true }).click();
  await expect
    .poll(() => page.locator("#sleep-select option").count())
    .toBeGreaterThan(2);
  await page.locator("#session-gap").fill("60");
  await page.getByRole("button", { name: "Применить", exact: true }).click();
  await expect(page.locator("#sleep-select option")).toHaveCount(2);
  await page.locator("#technical > summary").click();
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

test("reopening details preserves the selected damaged record", async ({
  page,
}) => {
  await open(
    page,
    "mixed.zip",
    archive({ "20010203": fixture("clock"), "20010204": fixture("truncated") }),
  );
  await page.locator("#day-next").click();
  await expect(page.locator("#day-title")).toContainText("4 февраля");
  await expect(page.locator("#day-time")).toContainText("повреждены");
  await page.locator("#technical > summary").click();
  await page.locator("#technical > summary").click();
  await expect(page.locator("#day-title")).toContainText("4 февраля");
  await expect(page.locator("#day-time")).toContainText("повреждены");
  const doc = JSON.parse((await download(page, "day", "json")).toString());
  expect(
    doc.sources.every(
      (source: { date: string }) => source.date === "2001-02-04",
    ),
  ).toBe(true);
});
