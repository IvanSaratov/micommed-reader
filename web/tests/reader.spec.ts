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
const archive = (entries: Record<string, Uint8Array>) =>
  Buffer.from(zipSync(entries));
async function openReader(page: Page, bytes: Buffer) {
  await page.goto("/");
  const asset = readdirSync(join(root, "web/dist/assets")).find((name) =>
    /^worker-.*\.js$/.test(name),
  )!;
  return page.evaluate(
    async ({ asset, bytes }) => {
      const worker = new Worker("/assets/" + asset, { type: "module" });
      let id = 0;
      const pending = new Map<
        number,
        {
          resolve: (value: any) => void;
          reject: (error: Error) => void;
          chunks: Uint8Array[];
        }
      >();
      let resolveReady!: () => void, rejectReady!: (error: Error) => void;
      const ready = new Promise<void>((resolve, reject) => {
        resolveReady = resolve;
        rejectReady = reject;
      });
      worker.onmessage = ({ data }) => {
        if (data.type === "ready") {
          resolveReady();
          return;
        }
        if (data.type === "fatal") {
          rejectReady(new Error(data.error));
          return;
        }
        const request = pending.get(data.id);
        if (!request || data.type === "progress") return;
        if (data.type === "chunk") {
          request.chunks.push(data.bytes);
          return;
        }
        pending.delete(data.id);
        if (data.error) request.reject(new Error(data.error));
        else
          request.resolve(
            request.chunks.length
              ? new Blob(request.chunks as BlobPart[])
              : data.value,
          );
      };
      const call = async (
        action: string,
        args: Record<string, unknown> = {},
      ) => {
        await ready;
        return new Promise<any>((resolve, reject) => {
          const next = ++id;
          pending.set(next, { resolve, reject, chunks: [] });
          worker.postMessage({ id: next, action, ...args });
        });
      };
      (window as any).testReader = call;
      return call("open", {
        files: [new File([new Uint8Array(bytes)], "records.zip")],
      });
    },
    { asset, bytes: Array.from(bytes) },
  );
}
async function call(
  page: Page,
  action: string,
  args: Record<string, unknown> = {},
) {
  return page.evaluate(
    async ({ action, args }) => {
      const value = await (window as any).testReader(action, args);
      return value instanceof Blob
        ? Array.from(new Uint8Array(await value.arrayBuffer()))
        : value;
    },
    { action, args },
  );
}
async function exported(page: Page, format: string, from = "", through = "") {
  return Buffer.from(await call(page, "export", { format, from, through }));
}

test("WASM preserves CLI JSON and all CSV tables, inclusive ranges and raw evidence", async ({
  page,
  context,
}) => {
  const bytes = archive({
    "HealData/20010203": fixture("clock"),
    "copy/20010203": fixture("clock"),
    "HealData/20010204": fixture("next-day"),
  });
  const { doc, source } = native(bytes);
  const index = await openReader(page, bytes);
  expect(index.period.therapy_minutes.value).toBe(31);
  const requests: string[] = [];
  context.on("request", (r) => requests.push(r.url()));
  expect(JSON.parse((await exported(page, "json")).toString())).toEqual(doc);
  const tables = unzipSync(await exported(page, "csv"));
  const directory = join(temp, "csv");
  const run = spawnSync(cli, ["export", "csv", "--out", directory, source], {
    encoding: "utf8",
  });
  expect([0, 3]).toContain(run.status);
  expect(Object.keys(tables).sort()).toEqual(readdirSync(directory).sort());
  for (const name of Object.keys(tables))
    expect(Buffer.from(tables[name])).toEqual(
      readFileSync(join(directory, name)),
    );
  expect(
    JSON.parse(
      (await exported(page, "json", "2001-02-03", "2001-02-03")).toString(),
    ),
  ).toEqual(native(bytes, "2001-02-03", "2001-02-03").doc);
  const empty = JSON.parse(
    (await exported(page, "json", "2001-03-01", "2001-03-02")).toString(),
  );
  expect(empty.days).toEqual([]);
  expect(empty.period.recalculated_ahi.value).toBeNull();
  const sessions = await call(page, "sessions", { gap: 60 });
  const split = await call(page, "sessions", { gap: 1 });
  expect(split.sessions.length).toBeGreaterThanOrEqual(
    sessions.sessions.length,
  );
  expect(requests).toEqual([]);
});
