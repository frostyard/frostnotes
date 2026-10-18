// Where the app keeps its own state on this device: local storage
// (ADR-0021). Reads and writes never throw, so a webview that refuses
// storage still runs, forgetting what it would have kept.

/** KeyValue is the app's view of local storage. */
export interface KeyValue {
  get(key: string): string | null;
  set(key: string, value: string): void;
  remove(key: string): void;
}

/** localStore returns local storage, or a store that keeps nothing where there is none. */
export function localStore(): KeyValue {
  const ls = (): Storage | null => {
    try {
      return globalThis.localStorage ?? null;
    } catch {
      return null;
    }
  };
  return {
    get: (key) => {
      try {
        return ls()?.getItem(key) ?? null;
      } catch {
        return null;
      }
    },
    set: (key, value) => {
      try {
        ls()?.setItem(key, value);
      } catch {
        // Full or refused: the value is not kept.
      }
    },
    remove: (key) => {
      try {
        ls()?.removeItem(key);
      } catch {
        // Refused: nothing to remove.
      }
    },
  };
}

/** memoryStore is a KeyValue in memory, for tests. */
export function memoryStore(init: Record<string, string> = {}): KeyValue & { data: Map<string, string> } {
  const data = new Map(Object.entries(init));
  return {
    data,
    get: (key) => data.get(key) ?? null,
    set: (key, value) => void data.set(key, value),
    remove: (key) => void data.delete(key),
  };
}
