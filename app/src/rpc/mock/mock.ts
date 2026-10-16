// MockTransport: an in-memory notesd for component tests and for running the
// UI in a browser (VITE_MOCK=1). It implements the protocol-1 methods with
// notesd's semantics (docs/specs/rpc-protocol.md, docs/specs/rpc-api.md):
// rpc.hello first, events only after events.subscribe, and vault.add and
// vault.remove emitting durable vault.added and vault.removed events.
import { byName } from "../../data/stores";
import { ErrorCode, type Event, PROTOCOL, type Vault } from "../gen/api";
import { RPCError, type Transport } from "../transport";

/** MockOptions tune the mock. */
export interface MockOptions {
  /** Milliseconds before each reply; 0 replies on the next microtask. */
  latency?: number;
  /** The server name rpc.hello reports. */
  server?: string;
  /** The clock for createdAt; the real one by default. */
  now?: () => Date;
}

type Params = Record<string, unknown>;

/** MockTransport serves vaults from memory through the Transport interface. */
export class MockTransport implements Transport {
  /** Every call, for assertions. */
  readonly calls: { method: string; params: unknown }[] = [];
  private readonly vaults: Vault[];
  private nextId: number;
  private seq = 0;
  private hello = false;
  private subscribed = false;
  private readonly eventHandlers = new Set<(event: Event) => void>();
  private readonly closeHandlers = new Set<(reason: string) => void>();
  private closed: string | undefined;

  constructor(
    vaults: Vault[] = [],
    private readonly opts: MockOptions = {},
  ) {
    this.vaults = vaults.map((v) => ({ ...v }));
    this.nextId = Math.max(0, ...vaults.map((v) => v.id)) + 1;
  }

  call<T>(method: string, params: unknown): Promise<T> {
    this.calls.push({ method, params });
    if (this.closed !== undefined) return Promise.reject(new Error(`notesd connection closed: ${this.closed}`));
    return this.later(() => this.dispatch(method, (params ?? {}) as Params) as T);
  }

  onEvent(handler: (event: Event) => void): () => void {
    this.eventHandlers.add(handler);
    return () => this.eventHandlers.delete(handler);
  }

  onClose(handler: (reason: string) => void): () => void {
    if (this.closed !== undefined) {
      handler(this.closed);
      return () => {};
    }
    this.closeHandlers.add(handler);
    return () => this.closeHandlers.delete(handler);
  }

  /** close simulates a lost connection. */
  close(reason: string): void {
    if (this.closed !== undefined) return;
    this.closed = reason;
    for (const h of this.closeHandlers) h(reason);
    this.closeHandlers.clear();
  }

  private later<T>(fn: () => T): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const run = () => {
        try {
          resolve(fn());
        } catch (err) {
          reject(err);
        }
      };
      if (this.opts.latency) setTimeout(run, this.opts.latency);
      else queueMicrotask(run);
    });
  }

  // emit delivers a durable event to a subscribed connection after the
  // reply that caused it, as notesd sends events after the commit.
  private emit(event: Omit<Event, "seq">): void {
    const e = { ...event, seq: ++this.seq } as Event;
    if (!this.subscribed) return;
    setTimeout(() => {
      if (this.closed !== undefined) return;
      for (const h of this.eventHandlers) h(e);
    }, this.opts.latency ?? 0);
  }

  private dispatch(method: string, p: Params): unknown {
    if (method === "rpc.hello") {
      if (p.protocol !== PROTOCOL) {
        throw new RPCError(
          ErrorCode.protocolMismatch,
          `server speaks protocol ${PROTOCOL}, client ${String(p.protocol)}`,
        );
      }
      this.hello = true;
      return { protocol: PROTOCOL, server: this.opts.server ?? "notesd mock" };
    }
    if (!this.hello) throw new RPCError(ErrorCode.notReady, "call rpc.hello first");
    switch (method) {
      case "events.subscribe":
        if (this.subscribed) throw new RPCError(ErrorCode.conflict, "already subscribed");
        this.subscribed = true;
        return { seq: this.seq, resync: false };
      case "vault.list":
        return this.sorted().map((v) => ({ ...v }));
      case "vault.add":
        return { ...this.add(p) };
      case "vault.remove": {
        const id = Number(p.id);
        const i = this.vaults.findIndex((v) => v.id === id);
        if (i < 0) throw new RPCError(ErrorCode.notFound, `vault ${id}: not found`);
        this.vaults.splice(i, 1);
        this.emit({ event: "vault.removed", data: { id } });
        return null;
      }
      default:
        throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
    }
  }

  private add(p: Params): Vault {
    const path = typeof p.path === "string" ? p.path.replace(/\/+$/, "") || "/" : "";
    if (!path.startsWith("/")) throw new RPCError(ErrorCode.invalidParams, `vault path "${path}" is not absolute`);
    const name = typeof p.name === "string" ? p.name.trim() : (path.split("/").pop() ?? "");
    if (name === "") throw new RPCError(ErrorCode.invalidParams, "vault name is empty");
    for (const v of this.vaults) {
      if (within(path, v.path) || within(v.path, path)) {
        throw new RPCError(ErrorCode.conflict, `vault "${path}" overlaps vault "${v.name}" at ${v.path}`);
      }
    }
    const vault: Vault = {
      id: this.nextId++,
      name,
      path,
      createdAt: (this.opts.now?.() ?? new Date()).toISOString(),
    };
    this.vaults.push(vault);
    this.emit({ event: "vault.added", data: { vault: { ...vault } } });
    return vault;
  }

  private sorted(): Vault[] {
    return [...this.vaults].sort(byName);
  }
}

// within reports whether path is dir or lies inside it.
function within(path: string, dir: string): boolean {
  return path === dir || path.startsWith(dir === "/" ? "/" : `${dir}/`);
}

/** mockVaults are the made-up vaults VITE_MOCK=1 shows. */
export function mockVaults(): Vault[] {
  return [
    { id: 1, name: "Notes", path: "/home/demo/Notes", createdAt: "2026-10-01T09:00:00Z" },
    { id: 2, name: "Work", path: "/home/demo/Documents/Work", createdAt: "2026-10-02T14:30:00Z" },
    { id: 3, name: "garden", path: "/home/demo/garden", createdAt: "2026-10-03T18:45:00Z" },
  ];
}
