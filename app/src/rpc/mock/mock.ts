// MockTransport: an in-memory notesd for component tests and for running the
// UI in a browser (VITE_MOCK=1). It implements the protocol-1 methods the
// app calls with notesd's semantics (docs/specs/rpc-protocol.md,
// docs/specs/rpc-api.md): rpc.hello first; events only after
// events.subscribe, durable ones kept and replayed after sinceSeq; the
// vault domain; the note domain over each vault's MockVault (vault.ts);
// and a sync domain for vaults that sync with no hub. As caller session
// (MockOptions.caller) it refuses what notesd refuses a browser inside
// lodge, and names a session's writes as notesd does.
import { byName } from "../../data/stores";
import {
  type CallerKind,
  ErrorCode,
  type Event,
  type FileBase,
  type Hello,
  METHOD_CALLERS,
  type Method,
  PROTOCOL,
  type Vault,
  type VaultConfigureParams,
  type VersionSource,
} from "../gen/api";
import { RPCError, type Transport } from "../transport";
import { mockFiles } from "./fixture";
import { type Author, MAX_UPLOAD, type MergeScript, MockVault } from "./vault";

export { defaultMerge, type MergeInput, type MergeOutput, type MergeScript, MockVault } from "./vault";

/** MockOptions tune the mock. */
export interface MockOptions {
  /** Milliseconds before each reply; 0 replies on the next microtask. */
  latency?: number;
  /** The server name rpc.hello reports. */
  server?: string;
  /** The clock for createdAt and versions; the real one by default. */
  now?: () => Date;
  /** Each vault's files by vault ID: a path and its content, or a folder as "path/". */
  files?: Record<number, Record<string, string | Uint8Array>>;
  /** Folders on the mock's disk besides the vaults and their parents, for vault.add's create. */
  folders?: string[];
  /** The connection's caller kind: local (the default), or session for the web app inside lodge. */
  caller?: CallerKind;
  /** A session's login, display name and node, as lodge names them. */
  login?: string;
  displayName?: string;
  node?: string;
}

type Params = Record<string, unknown>;

/** MockTransport serves vaults from memory through the Transport interface. */
export class MockTransport implements Transport {
  /** Every call, for assertions. */
  readonly calls: { method: string; params: unknown }[] = [];
  private readonly vaults: Vault[];
  private readonly contents = new Map<number, MockVault>();
  private nextVault: number;
  /** The folders on the mock's disk: its vaults', their parents', and MockOptions.folders. */
  private readonly folders = new Set<string>();
  private nextId = 1;
  private seq = 0;
  private readonly log: Event[] = [];
  private hello = false;
  private subscribed = false;
  private client = "";
  private script: MergeScript | undefined;
  private readonly eventHandlers = new Set<(event: Event) => void>();
  private readonly closeHandlers = new Set<(reason: string) => void>();
  private closed: string | undefined;

  constructor(
    vaults: Vault[] = [],
    private readonly opts: MockOptions = {},
  ) {
    this.vaults = vaults.map((v) => ({ ...v }));
    this.nextVault = Math.max(0, ...vaults.map((v) => v.id)) + 1;
    for (const f of [...this.vaults.map((v) => v.path), ...(opts.folders ?? [])]) this.addFolder(f);
    for (const v of this.vaults) this.contents.set(v.id, this.newVault(v.id, opts.files?.[v.id]));
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

  /** blobURL gives a file's bytes as a data: URL (ADR-0023). */
  blobURL(hash: string, ext: string): string {
    for (const v of this.contents.values()) {
      const bytes = v.blob(hash);
      if (bytes) return `data:${contentType(ext)};base64,${toBase64(bytes)}`;
    }
    return "";
  }

  /** close simulates a lost connection. */
  close(reason: string): void {
    if (this.closed !== undefined) return;
    this.closed = reason;
    for (const h of this.closeHandlers) h(reason);
    this.closeHandlers.clear();
  }

  /**
   * reopen makes the transport a new connection to the same notesd, after
   * close: hello and the subscription start again, and what changed
   * meanwhile is kept for a subscription's replay.
   */
  reopen(): void {
    this.closed = undefined;
    this.hello = false;
    this.subscribed = false;
    this.eventHandlers.clear();
    this.closeHandlers.clear();
  }

  /** vault returns a vault's contents, for tests that change it as another writer would. */
  vault(id: number): MockVault {
    const v = this.contents.get(id);
    if (!v) throw new Error(`no vault ${id}`);
    return v;
  }

  /** scriptMerges makes script decide every merge of a stale write until it is replaced; undefined restores the default. */
  scriptMerges(script: MergeScript | undefined): void {
    this.script = script;
  }

  private newVault(id: number, files?: Record<string, string | Uint8Array>): MockVault {
    return new MockVault(
      id,
      {
        now: () => this.now(),
        nextId: () => this.nextId++,
        emit: (e) => this.emit(e),
        merge: (m) => this.script?.(m),
      },
      files,
    );
  }

  private now(): Date {
    return this.opts.now?.() ?? new Date();
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

  // emit keeps a durable event and delivers it to a subscribed connection
  // after the reply that caused it, as notesd sends events after the commit.
  private emit(event: Omit<Event, "seq">): void {
    const e = { ...event, seq: ++this.seq } as Event;
    this.log.push(e);
    if (this.subscribed) this.deliver([e]);
  }

  private deliver(events: Event[]): void {
    setTimeout(() => {
      if (this.closed !== undefined) return;
      for (const e of events) for (const h of this.eventHandlers) h(e);
    }, this.opts.latency ?? 0);
  }

  private get caller(): CallerKind {
    return this.opts.caller ?? "local";
  }

  private dispatch(method: string, p: Params): unknown {
    const admits: readonly CallerKind[] | undefined = METHOD_CALLERS[method as Method];
    if (!admits?.includes(this.caller)) {
      throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
    }
    if (method === "rpc.hello") {
      if (p.protocol !== PROTOCOL) {
        throw new RPCError(
          ErrorCode.protocolMismatch,
          `server speaks protocol ${PROTOCOL}, client ${String(p.protocol)}`,
        );
      }
      this.hello = true;
      this.client = typeof p.client === "string" ? p.client : "";
      const hello: Hello = { protocol: PROTOCOL, server: this.opts.server ?? "notesd mock", caller: this.caller };
      if (this.caller === "session") {
        if (this.opts.login) hello.login = this.opts.login;
        if (this.opts.displayName) hello.displayName = this.opts.displayName;
        if (this.opts.node) hello.node = this.opts.node;
      }
      return hello;
    }
    if (!this.hello) throw new RPCError(ErrorCode.notReady, "call rpc.hello first");
    if (method === "rpc.ping") return null;
    const [domain] = method.split(".");
    switch (domain) {
      case "events":
        return this.events(method, p);
      case "vault":
        return this.vaultDomain(method, p);
      case "note":
        return this.note(method, p);
      case "sync":
        return this.sync(method, p);
    }
    throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
  }

  private events(method: string, p: Params): unknown {
    if (method !== "events.subscribe") throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
    if (this.subscribed) throw new RPCError(ErrorCode.conflict, "already subscribed");
    this.subscribed = true;
    const since = p.sinceSeq;
    if (typeof since !== "number" || since === this.seq) return { seq: this.seq, resync: false };
    if (since < 0 || since > this.seq) return { seq: this.seq, resync: true };
    this.deliver(this.log.filter((e) => (e.seq ?? 0) > since));
    return { seq: this.seq, resync: false };
  }

  private vaultDomain(method: string, p: Params): unknown {
    switch (method) {
      case "vault.list":
        return [...this.vaults].sort(byName).map((v) => ({ ...v }));
      case "vault.add":
        return { ...this.add(p) };
      case "vault.remove": {
        const id = Number(p.id);
        const i = this.vaults.findIndex((v) => v.id === id);
        if (i < 0) throw new RPCError(ErrorCode.notFound, `vault ${id}: not found`);
        this.vaults.splice(i, 1);
        this.contents.delete(id);
        this.emit({ event: "vault.removed", data: { id } });
        return null;
      }
      case "vault.status":
      case "vault.reindex":
        return {
          vaultId: Number(p.id),
          state: "idle",
          done: 0,
          total: 0,
          network: false,
          ...this.target(p.id).counts(),
        };
      case "vault.settings":
        return this.target(p.id).getSettings();
      case "vault.configure": {
        const { id, ...changes } = p as unknown as VaultConfigureParams;
        return this.target(id).configure(changes);
      }
    }
    throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
  }

  private note(method: string, p: Params): unknown {
    const v = this.target(p.vaultId);
    const who = this.author(p);
    const path = String(p.path ?? "");
    switch (method) {
      case "note.files":
        return v.listing();
      case "note.read":
        return v.read(path);
      case "note.list":
        return v.list(p as { folder?: string; tag?: string; property?: string; value?: string });
      case "note.search":
        return v.search(String(p.query ?? ""), typeof p.limit === "number" ? p.limit : 20);
      case "note.links":
        return v.links(path);
      case "note.backlinks":
        return v.backlinks(path);
      case "note.create":
        return v.create(path, new TextEncoder().encode(String(p.content ?? "")), who);
      case "note.write":
        return v.write(path, String(p.base ?? ""), String(p.content ?? ""), who);
      case "note.move":
        return v.move(path, String(p.to ?? ""));
      case "note.delete":
        v.delete(path, String(p.base ?? ""), who);
        return null;
      case "note.versions":
        return v.versionList(path, typeof p.limit === "number" ? p.limit : 50);
      case "note.readVersion":
        return v.readVersion(Number(p.version));
      case "note.revert":
        return v.revert(path, Number(p.version), String(p.base ?? ""), who);
      case "note.resolve":
        return v.resolutions(String(p.from ?? ""), Array.isArray(p.targets) ? p.targets.map(String) : []);
      case "note.render":
        return { html: v.render(path, typeof p.content === "string" ? p.content : undefined) };
      case "note.upload": {
        const data = fromBase64(String(p.data ?? ""));
        if (data.length > MAX_UPLOAD) {
          throw new RPCError(ErrorCode.invalidParams, `an upload is at most ${MAX_UPLOAD} bytes`);
        }
        return v.create(path, data, who);
      }
      case "note.tags":
        return v.tagCounts();
      case "note.properties":
        return v.propertyCounts();
      case "note.daily":
        return v.daily(typeof p.date === "string" ? new Date(`${p.date}T12:00:00Z`) : this.now(), who);
      case "note.template":
        return {
          content: v.template(String(p.template ?? ""), {
            title: typeof p.title === "string" ? p.title : "",
            at: typeof p.at === "string" ? new Date(p.at) : this.now(),
          }),
        };
      case "note.mkdir":
        v.mkdir(path);
        return null;
      case "note.moveFolder":
        return v.moveFolder(path, String(p.to ?? ""));
      case "note.deleteFolder":
        return v.deleteFolder(path, (p.files ?? []) as FileBase[], who);
    }
    throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
  }

  // No mock vault syncs: sync.status lists none, and the rest answer as
  // notesd does for a vault without a hub.
  private sync(method: string, p: Params): unknown {
    if (method === "sync.status" || method === "sync.hubs") return [];
    if (p.vaultId !== undefined) this.target(p.vaultId);
    throw new RPCError(ErrorCode.notFound, `vault ${String(p.vaultId)} does not sync with a hub`);
  }

  private target(id: unknown): MockVault {
    const v = this.contents.get(Number(id));
    if (!v) throw new RPCError(ErrorCode.notFound, `vault ${String(id)} does not exist`);
    return v;
  }

  // author is who makes a write: a session's connection names it, else
  // source editor unless the writer says agent, and the client name given
  // to rpc.hello unless it names one.
  private author(p: Params): Author {
    if (this.caller === "session") return { source: "editor", name: `${this.opts.node ?? ""} (web)` };
    const source = (p.source ?? "editor") as VersionSource;
    if (source !== "editor" && source !== "agent") {
      throw new RPCError(ErrorCode.invalidParams, `source "${source}": a write's source is editor or agent`);
    }
    return { source, name: typeof p.author === "string" ? p.author.trim() : this.client };
  }

  // addFolder records a folder on the mock's disk, with its parents.
  private addFolder(path: string): void {
    for (let p = path; p !== ""; p = p.slice(0, p.lastIndexOf("/"))) this.folders.add(p);
    this.folders.add("/");
  }

  private add(p: Params): Vault {
    const path = typeof p.path === "string" ? p.path.replace(/\/+$/, "") || "/" : "";
    if (!path.startsWith("/")) throw new RPCError(ErrorCode.invalidParams, `vault path "${path}" is not absolute`);
    // With create, notesd makes the folder: its parent must exist and it must not.
    if (p.create === true) {
      const parent = path.slice(0, path.lastIndexOf("/")) || "/";
      if (path === "/") throw new RPCError(ErrorCode.invalidParams, `vault path "${path}" names no folder to make`);
      if (!this.folders.has(parent)) {
        throw new RPCError(
          ErrorCode.invalidParams,
          `the folder to hold the vault: ${parent}: no such file or directory`,
        );
      }
      if (this.folders.has(path)) throw new RPCError(ErrorCode.conflict, `${path} already exists`);
    }
    const name = typeof p.name === "string" ? p.name.trim() : (path.split("/").pop() ?? "");
    if (name === "") throw new RPCError(ErrorCode.invalidParams, "vault name is empty");
    for (const v of this.vaults) {
      if (within(path, v.path) || within(v.path, path)) {
        throw new RPCError(ErrorCode.conflict, `vault "${path}" overlaps vault "${v.name}" at ${v.path}`);
      }
    }
    this.addFolder(path);
    const vault: Vault = { id: this.nextVault++, name, path, createdAt: this.now().toISOString() };
    this.vaults.push(vault);
    this.contents.set(vault.id, this.newVault(vault.id));
    this.emit({ event: "vault.added", data: { vault: { ...vault } } });
    return vault;
  }
}

// within reports whether path is dir or lies inside it.
function within(path: string, dir: string): boolean {
  return path === dir || path.startsWith(dir === "/" ? "/" : `${dir}/`);
}

/** CONTENT_TYPES name a file's type by its extension, as notesd's blob servers do (docs/design/app.md#attachments). */
const CONTENT_TYPES: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  avif: "image/avif",
  bmp: "image/bmp",
  svg: "image/svg+xml",
  mp3: "audio/mpeg",
  wav: "audio/wav",
  m4a: "audio/mp4",
  "3gp": "audio/3gpp",
  flac: "audio/flac",
  ogg: "audio/ogg",
  oga: "audio/ogg",
  opus: "audio/opus",
  mp4: "video/mp4",
  webm: "video/webm",
  ogv: "video/ogg",
  mov: "video/quicktime",
  mkv: "video/x-matroska",
  pdf: "application/pdf",
  md: "text/markdown; charset=utf-8",
};

/** contentType is the content type notesd serves a file of extension ext as. */
export function contentType(ext: string): string {
  return CONTENT_TYPES[ext.toLowerCase()] ?? "application/octet-stream";
}

function toBase64(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function fromBase64(data: string): Uint8Array {
  let s: string;
  try {
    s = atob(data);
  } catch {
    throw new RPCError(ErrorCode.invalidParams, "data is not base64");
  }
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

/** mockVaults are the made-up vaults VITE_MOCK=1 shows. */
export function mockVaults(): Vault[] {
  return [
    { id: 1, name: "Notes", path: "/home/demo/Notes", createdAt: "2026-10-01T09:00:00Z" },
    { id: 2, name: "Work", path: "/home/demo/Documents/Work", createdAt: "2026-10-02T14:30:00Z" },
    { id: 3, name: "garden", path: "/home/demo/garden", createdAt: "2026-10-03T18:45:00Z" },
  ];
}

export { mockFiles };
