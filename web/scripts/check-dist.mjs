import { readdirSync, lstatSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const dist = fileURLToPath(new URL("../dist/", import.meta.url));
const allowed =
  /^(index\.html|THIRD-PARTY-NOTICES\.txt|wasm\/(healdata\.wasm|wasm_exec\.js)|assets\/[A-Za-z0-9_-]+\.(js|css|woff2))$/;
let count = 0;
function check(directory, prefix = "") {
  for (const name of readdirSync(directory)) {
    const path = join(directory, name);
    const relative = prefix + name;
    const stat = lstatSync(path);
    if (stat.isDirectory()) check(path, relative + "/");
    else {
      if (!stat.isFile() || !allowed.test(relative))
        throw new Error("Посторонний файл в сборке: " + relative);
      if (/\.(js|css|html)$/.test(relative)) {
        const source = readFileSync(path, "utf8");
        if (
          /data\/private|research\/fixtures|\/Users\/|\/home\/|sourceMappingURL=/.test(
            source,
          )
        ) {
          throw new Error(
            "Локальный путь или отладочные материалы в сборке: " + relative,
          );
        }
      }
      count++;
    }
  }
}
check(dist);
console.log(
  `Проверены ${count} публичных ресурсов; исходные записи и отчёты в dist не входят.`,
);
