import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
const root = fileURLToPath(new URL("../../", import.meta.url));
const assets = join(root, "web/public/wasm");
mkdirSync(assets, { recursive: true });
const goroot = execFileSync("go", ["env", "GOROOT"], {
  encoding: "utf8",
}).trim();
execFileSync(
  "go",
  [
    "build",
    "-trimpath",
    "-o",
    join(assets, "healdata.wasm"),
    "./cmd/healdata-wasm",
  ],
  {
    cwd: root,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  },
);
copyFileSync(
  join(goroot, "lib/wasm/wasm_exec.js"),
  join(assets, "wasm_exec.js"),
);
// Лицензии входят в статическую публикацию вместе со шрифтом, графиками и рантаймом.
const notices = [
  ["HEALDATA", join(root, "LICENSE")],
  [
    "Go",
    existsSync(join(goroot, "LICENSE"))
      ? join(goroot, "LICENSE")
      : join(goroot, "../LICENSE"),
  ],
  ["Apache ECharts", join(root, "web/node_modules/echarts/LICENSE")],
  ["Apache ECharts NOTICE", join(root, "web/node_modules/echarts/NOTICE")],
  ["d3 (ECharts)", join(root, "web/node_modules/echarts/licenses/LICENSE-d3")],
  ["zrender", join(root, "web/node_modules/zrender/LICENSE")],
  ["Inter", join(root, "web/node_modules/@fontsource-variable/inter/LICENSE")],
];
writeFileSync(
  join(root, "web/public/THIRD-PARTY-NOTICES.txt"),
  notices
    .map(([name, path]) => name + "\n\n" + readFileSync(path, "utf8"))
    .join("\n\n---\n\n"),
);
