import ReaderWorker from "./worker?worker";

interface Pending {
  resolve: (v: unknown) => void;
  reject: (e: Error) => void;
  chunks: Uint8Array[];
}
export class Reader {
  private worker: Worker;
  private pending = new Map<number, Pending>();
  private nextID = 0;
  private failure: Error | undefined;
  private ready: Promise<void>;
  private rejectReady: (e: Error) => void = () => {};
  constructor(progress: (done: number, total: number) => void) {
    this.worker = new ReaderWorker();
    this.ready = new Promise((resolve, reject) => {
      this.rejectReady = reject;
      this.worker.onmessage = ({ data }) => {
        if (data.type === "ready") {
          resolve();
          return;
        }
        if (data.type === "fatal") {
          this.fail(new Error(data.error));
          return;
        }
        const request = this.pending.get(data.id);
        if (!request) return;
        if (data.type === "progress") {
          progress(data.done, data.total);
          return;
        }
        if (data.type === "chunk") {
          request.chunks.push(data.bytes);
          return;
        }
        this.pending.delete(data.id);
        if (data.error) request.reject(new Error(data.error));
        else
          request.resolve(
            request.chunks.length
              ? new Blob(request.chunks as BlobPart[])
              : data.value,
          );
      };
      this.worker.onerror = (event) =>
        this.fail(
          new Error(event.message || "Не удалось запустить чтение файлов"),
        );
    });
  }
  private fail(error: Error) {
    this.failure = error;
    this.rejectReady(error);
    for (const request of this.pending.values()) request.reject(error);
    this.pending.clear();
  }
  async call<T>(
    action: string,
    args: Record<string, unknown> = {},
  ): Promise<T> {
    await this.ready;
    if (this.failure) throw this.failure;
    return new Promise<T>((resolve, reject) => {
      const id = ++this.nextID;
      this.pending.set(id, {
        resolve: (value) => resolve(value as T),
        reject,
        chunks: [],
      });
      this.worker.postMessage({ id, action, ...args });
    });
  }
  close() {
    this.worker.terminate();
    this.fail(new Error("Чтение отменено"));
  }
}
