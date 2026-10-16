// The line-level contract between the generated Client (gen/api.ts) and a
// connection to notesd. JSON-RPC multiplexing happens here, in the webview;
// the Tauri bridge and the dev gateway only move lines
// (docs/specs/rpc-protocol.md).
import type { Event } from "./gen/api";

export interface Transport {
  call<T>(method: string, params: unknown): Promise<T>;
  /** Subscribe to server events; returns an unsubscribe function. */
  onEvent(handler: (event: Event) => void): () => void;
  /** Called once when the connection ends. */
  onClose(handler: (reason: string) => void): () => void;
}

/** A JSON-RPC error returned by notesd; code is an ErrorCode. */
export class RPCError extends Error {
  constructor(
    readonly code: number,
    message: string,
    readonly data?: unknown,
  ) {
    super(message);
    this.name = "RPCError";
  }
}

interface Pending {
  resolve: (value: unknown) => void;
  reject: (reason: Error) => void;
}

interface Incoming {
  id?: number | string | null;
  method?: string;
  params?: unknown;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

/**
 * JsonRpcSession implements Transport over any line channel: the caller
 * supplies how to send a line and feeds received lines to receive().
 */
export class JsonRpcSession implements Transport {
  private nextId = 0;
  private readonly pending = new Map<number, Pending>();
  private readonly eventHandlers = new Set<(event: Event) => void>();
  private readonly closeHandlers = new Set<(reason: string) => void>();
  private closedReason: string | undefined;

  constructor(private readonly sendLine: (line: string) => Promise<void>) {}

  call<T>(method: string, params: unknown): Promise<T> {
    if (this.closedReason !== undefined) {
      return Promise.reject(new Error(`notesd connection closed: ${this.closedReason}`));
    }
    const id = ++this.nextId;
    const line = JSON.stringify({ jsonrpc: "2.0", id, method, params: params ?? {} });
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject });
      this.sendLine(line).catch((err: unknown) => {
        this.pending.delete(id);
        reject(err instanceof Error ? err : new Error(String(err)));
      });
    });
  }

  onEvent(handler: (event: Event) => void): () => void {
    this.eventHandlers.add(handler);
    return () => this.eventHandlers.delete(handler);
  }

  onClose(handler: (reason: string) => void): () => void {
    if (this.closedReason !== undefined) {
      handler(this.closedReason);
      return () => {};
    }
    this.closeHandlers.add(handler);
    return () => this.closeHandlers.delete(handler);
  }

  /** Feed one line received from notesd. */
  receive(line: string): void {
    let msg: Incoming;
    try {
      msg = JSON.parse(line) as Incoming;
    } catch {
      this.close(`malformed message from notesd: ${line.slice(0, 120)}`);
      return;
    }
    if (msg.id === undefined || msg.id === null) {
      if (msg.method === "event" && msg.params) {
        for (const h of this.eventHandlers) h(msg.params as Event);
      }
      return;
    }
    if (typeof msg.id !== "number") return;
    const p = this.pending.get(msg.id);
    if (!p) return;
    this.pending.delete(msg.id);
    if (msg.error) p.reject(new RPCError(msg.error.code, msg.error.message, msg.error.data));
    else p.resolve(msg.result ?? null);
  }

  /** Fail pending calls and notify close handlers; idempotent. */
  close(reason: string): void {
    if (this.closedReason !== undefined) return;
    this.closedReason = reason;
    for (const p of this.pending.values()) p.reject(new Error(`notesd connection closed: ${reason}`));
    this.pending.clear();
    for (const h of this.closeHandlers) h(reason);
    this.closeHandlers.clear();
  }
}
