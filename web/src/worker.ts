/// <reference lib="webworker" />
declare class FileReaderSync {
  readAsArrayBuffer(blob: Blob): ArrayBuffer;
}
declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

const scope = globalThis as unknown as DedicatedWorkerGlobalScope & {
  Go: typeof Go;
  healdataCall: (request: string) => string;
  healdataRead: (file: number, offset: number, length: number) => Uint8Array;
  healdataReady: () => void;
  healdataProgress: (done: number, total: number) => void;
  healdataChunk: (bytes: Uint8Array) => void;
};
let files: File[] = [];
let activeID = 0;
let cached: { file: number; start: number; bytes: Uint8Array } | undefined;
const reader = new FileReaderSync();
scope.healdataRead = (file, offset, length) => {
  if (
    !cached ||
    cached.file !== file ||
    offset < cached.start ||
    offset + length > cached.start + cached.bytes.length
  ) {
    const end = Math.min(
      files[file].size,
      offset + Math.max(length, 256 * 1024),
    );
    cached = {
      file,
      start: offset,
      bytes: new Uint8Array(
        reader.readAsArrayBuffer(files[file].slice(offset, end)),
      ),
    };
  }
  return cached.bytes.subarray(
    offset - cached.start,
    offset - cached.start + length,
  );
};
scope.healdataProgress = (done, total) =>
  scope.postMessage({ type: "progress", id: activeID, done, total });
scope.healdataChunk = (bytes) =>
  scope.postMessage({ type: "chunk", id: activeID, bytes }, [bytes.buffer]);
scope.healdataReady = () => scope.postMessage({ type: "ready" });
scope.onmessage = ({ data }) => {
  activeID = data.id;
  try {
    if (data.action === "open") {
      files = data.files;
      cached = undefined;
      data.files = files.map((file) => ({
        name: file.name,
        size: file.size,
        modified: file.lastModified,
      }));
    }
    const response = JSON.parse(scope.healdataCall(JSON.stringify(data)));
    scope.postMessage({ type: "result", id: data.id, ...response });
  } catch (error) {
    scope.postMessage({ type: "result", id: data.id, error: String(error) });
  }
};
async function boot() {
  // Стандартный рантайм Go загружается с того же адреса, что и WASM этой сборки.
  const runtimeURL = new URL(
    /* @vite-ignore */ "../wasm/wasm_exec.js",
    import.meta.url,
  );
  await import(/* @vite-ignore */ runtimeURL.href);
  const go = new scope.Go();
  const wasm = await fetch(
    new URL(/* @vite-ignore */ "../wasm/healdata.wasm", import.meta.url),
  );
  if (!wasm.ok) throw new Error("Не удалось загрузить модуль чтения файлов");
  const { instance } = await WebAssembly.instantiate(
    await wasm.arrayBuffer(),
    go.importObject,
  );
  void go
    .run(instance)
    .catch((error) =>
      scope.postMessage({ type: "fatal", error: String(error) }),
    );
}
void boot().catch((error) =>
  scope.postMessage({ type: "fatal", error: String(error) }),
);
