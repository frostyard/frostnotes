// MockVault: one vault of MockTransport, in memory, with notesd's semantics
// for the note and vault domains (docs/specs/rpc-api.md;
// docs/design/vault.md#writes): every write names its base, a stale base
// merges, a base that is no version is refused, every change keeps a
// version and emits its event. Its merges are scripted: a test supplies
// what a merge gives, and without a script the newer side stands.
import {
  ErrorCode,
  type Event,
  type FileKind,
  type FolderDeleteResult,
  type FolderMoveResult,
  type Link,
  type MoveResult,
  type Note,
  type NoteEntry,
  type PropertyCount,
  type Resolution,
  type SearchHit,
  type TagCount,
  type VaultFiles,
  type VaultSettings,
  type Version,
  type VersionSource,
  type WriteResult,
} from "../gen/api";
import { RPCError } from "../transport";
import { sha256 } from "./sha256";

/** MergeInput is what a write with a stale base merges. */
export interface MergeInput {
  path: string;
  base: string;
  head: string;
  incoming: string;
}

/** MergeOutput is a merge's text and how many hunks conflicted. */
export interface MergeOutput {
  content: string;
  conflicts: number;
}

/** MergeScript decides a merge; undefined leaves it to the default. */
export type MergeScript = (m: MergeInput) => MergeOutput | undefined;

/** Author is who makes a change. */
export interface Author {
  source: VersionSource;
  name: string;
}

/** VaultEnv is what a vault shares with its transport. */
export interface VaultEnv {
  now(): Date;
  nextId(): number;
  emit(event: Omit<Event, "seq">): void;
  merge(m: MergeInput): MergeOutput | undefined;
}

interface Entry {
  id: number;
  path: string;
  data: Uint8Array;
  hash: string;
  modifiedAt: string;
}

interface StoredVersion extends Version {
  fileId: number;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/** MAX_UPLOAD is note.upload's limit, in bytes (ADR-0023). */
export const MAX_UPLOAD = 10 << 20;

const DEFAULT_SETTINGS: VaultSettings = {
  dailyFolder: "",
  dailyFormat: "YYYY-MM-DD",
  dailyTemplate: "",
  templateFolder: "",
  templateDateFormat: "YYYY-MM-DD",
  templateTimeFormat: "HH:mm",
  newNoteLocation: "root",
  newNoteFolder: "",
  attachmentFolder: "/",
  linkFormat: "shortest",
  wikilinks: true,
  updateLinks: false,
  ignore: [],
  sources: Object.fromEntries(
    [
      "dailyFolder",
      "dailyFormat",
      "dailyTemplate",
      "templateFolder",
      "templateDateFormat",
      "templateTimeFormat",
      "newNoteLocation",
      "newNoteFolder",
      "attachmentFolder",
      "linkFormat",
      "wikilinks",
      "updateLinks",
      "ignore",
    ].map((k) => [k, "default"]),
  ),
};

const fail = (code: number, message: string): never => {
  throw new RPCError(code, message);
};

/** checkPath refuses what notesd's path guard refuses for the note domain. */
export function checkPath(p: unknown): string {
  if (typeof p !== "string" || p === "") return fail(ErrorCode.invalidParams, "the path is empty");
  if (p.startsWith("/")) return fail(ErrorCode.invalidParams, `${p} is absolute; paths are relative to the vault`);
  for (const seg of p.split("/")) {
    if (seg === "" || seg === "." || seg === "..") return fail(ErrorCode.invalidParams, `${p} has a "${seg}" segment`);
    if (seg.startsWith(".")) return fail(ErrorCode.invalidParams, `${p} has a hidden segment "${seg}"`);
  }
  return p;
}

const kindOf = (p: string): FileKind => (p.endsWith(".md") ? "note" : "attachment");
const base = (p: string) => p.slice(p.lastIndexOf("/") + 1);
const dir = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");
const title = (p: string) => (kindOf(p) === "note" ? base(p).slice(0, -3) : base(p));

function parents(p: string): string[] {
  const out: string[] = [];
  for (let d = dir(p); d !== ""; d = dir(d)) out.push(d);
  return out;
}

/** MockVault holds one vault's files, folders, versions and settings. */
export class MockVault {
  private readonly files = new Map<string, Entry>();
  private readonly folders = new Set<string>();
  private readonly blobs = new Map<string, Uint8Array>();
  private readonly versions: StoredVersion[] = [];
  private settings: VaultSettings = structuredClone(DEFAULT_SETTINGS);

  constructor(
    readonly id: number,
    private readonly env: VaultEnv,
    files: Record<string, string | Uint8Array> = {},
  ) {
    const who: Author = { source: "outside", name: "" };
    for (const [p, content] of Object.entries(files)) {
      if (p.endsWith("/")) this.addFolder(p.slice(0, -1));
      else this.put(checkPath(p), typeof content === "string" ? encoder.encode(content) : content, who, false);
    }
  }

  // --- what tests and the transport read directly

  /** text returns a file's content as note.read shows it, or undefined. */
  text(p: string): string | undefined {
    const e = this.files.get(p);
    return e && decoder.decode(e.data);
  }

  /** blob returns a hash's bytes, or undefined. */
  blob(hash: string): Uint8Array | undefined {
    return this.blobs.get(hash);
  }

  // --- the note domain

  listing(): VaultFiles {
    const folders = new Set(this.folders);
    const files = [...this.files.values()].sort((a, b) => cmp(a.path, b.path));
    for (const e of files) for (const d of parents(e.path)) folders.add(d);
    return {
      files: files.map((e) => ({
        path: e.path,
        kind: kindOf(e.path),
        hash: e.hash,
        size: e.data.length,
        modifiedAt: e.modifiedAt,
      })),
      folders: [...folders].sort(cmp),
    };
  }

  read(p: string): Note {
    const e = this.entry(p);
    const content = decoder.decode(e.data);
    const note: Note = {
      path: e.path,
      title: title(e.path),
      hash: e.hash,
      modifiedAt: e.modifiedAt,
      content,
      props: {},
      aliases: [],
      tags: [],
    };
    if (kindOf(e.path) === "note") {
      const fm = frontmatter(content);
      note.props = fm;
      note.aliases = list(fm.aliases);
      note.tags = [...list(fm.tags).map((t) => t.replace(/^#/, "")), ...inlineTags(content)];
    }
    return note;
  }

  create(p: string, data: Uint8Array, who: Author): WriteResult {
    checkPath(p);
    if (this.files.has(p)) fail(ErrorCode.conflict, `${p} exists`);
    if (this.folders.has(p)) fail(ErrorCode.conflict, `${p} is a folder`);
    const e = this.put(p, data, who, true);
    return { path: p, hash: e.hash, merged: false, conflicts: 0 };
  }

  write(p: string, baseHash: string, content: string, who: Author): WriteResult {
    const e = this.entry(p);
    if (baseHash === e.hash) {
      const next = this.put(p, encoder.encode(content), who, true);
      return { path: p, hash: next.hash, merged: false, conflicts: 0 };
    }
    const baseBytes = this.versions.some((v) => v.fileId === e.id && v.hash === baseHash) && this.blobs.get(baseHash);
    if (!baseBytes) {
      return fail(ErrorCode.conflict, `${p}: the base ${baseHash} is no version of the file; read it again`);
    }
    const head = decoder.decode(e.data);
    const input: MergeInput = { path: p, base: decoder.decode(baseBytes), head, incoming: content };
    const merged = this.env.merge(input) ?? defaultMerge(input);
    const last = this.versionsOf(e.id)[0];
    let conflictVersion: number | undefined;
    if (merged.conflicts > 0) {
      conflictVersion = this.keep(e, encoder.encode(content), who, true).id;
    }
    const next = merged.content === head ? e : this.put(p, encoder.encode(merged.content), who, true);
    return {
      path: p,
      hash: next.hash,
      merged: true,
      content: merged.content,
      conflicts: merged.conflicts,
      conflictVersion,
      mergedSource: last?.source,
      mergedAuthor: last?.author,
    };
  }

  move(from: string, to: string): MoveResult {
    checkPath(to);
    const e = this.entry(from);
    if (from === to) fail(ErrorCode.invalidParams, `${from} is already at ${to}`);
    if (this.files.has(to) || this.folders.has(to)) fail(ErrorCode.conflict, `${to} exists`);
    this.files.delete(from);
    e.path = to;
    this.files.set(to, e);
    this.env.emit({ event: "note.moved", data: { vaultId: this.id, from, to } });
    return { path: to, rewritten: [] };
  }

  delete(p: string, baseHash: string, who: Author): void {
    const e = this.entry(p);
    if (baseHash !== e.hash) fail(ErrorCode.conflict, `${p} changed since "${baseHash}", and an edit beats a delete`);
    this.remove(e, who);
  }

  versionList(p: string, limit = 50): Version[] {
    checkPath(p);
    const id = this.files.get(p)?.id ?? this.versions.findLast((v) => v.path === p)?.fileId;
    if (id === undefined) return fail(ErrorCode.notFound, `${p} does not exist`);
    return this.versionsOf(id)
      .slice(0, limit)
      .map(({ fileId: _, ...v }) => v);
  }

  readVersion(id: number): { version: Version; content: string } {
    const v = this.versions.find((x) => x.id === id);
    if (!v) return fail(ErrorCode.notFound, `version ${id} does not exist in vault ${this.id}`);
    const { fileId: _, ...version } = v;
    const bytes = v.hash === "" ? undefined : this.blobs.get(v.hash);
    return { version, content: bytes ? decoder.decode(bytes) : "" };
  }

  revert(p: string, id: number, baseHash: string, who: Author): WriteResult {
    const { content } = this.readVersion(id);
    if (!this.files.has(p)) return this.create(p, encoder.encode(content), who);
    return this.write(p, baseHash, content, who);
  }

  mkdir(p: string): void {
    p = checkPath(p.replace(/^\/+|\/+$/g, ""));
    if (this.files.has(p)) fail(ErrorCode.invalidParams, `${p} is a file`);
    this.addFolder(p);
  }

  moveFolder(from: string, to: string): FolderMoveResult {
    from = checkPath(from.replace(/^\/+|\/+$/g, ""));
    to = checkPath(to.replace(/^\/+|\/+$/g, ""));
    if (to === from || to.startsWith(`${from}/`)) fail(ErrorCode.invalidParams, `${from} cannot move to ${to}`);
    if (!this.listing().folders.includes(from)) fail(ErrorCode.notFound, `${from} does not exist`);
    if (this.files.has(to) || this.listing().folders.includes(to)) fail(ErrorCode.conflict, `${to} exists`);
    const moved: string[] = [];
    for (const e of [...this.files.values()].filter((x) => x.path.startsWith(`${from}/`))) {
      const next = to + e.path.slice(from.length);
      this.move(e.path, next);
      moved.push(next);
    }
    for (const f of [...this.folders]) {
      if (f === from || f.startsWith(`${from}/`)) {
        this.folders.delete(f);
        this.folders.add(to + f.slice(from.length));
      }
    }
    for (const d of parents(`${to}/x`)) this.folders.add(d);
    return { path: to, moved: moved.sort(cmp), rewritten: [] };
  }

  deleteFolder(folder: string, listed: { path: string; base: string }[], who: Author): FolderDeleteResult {
    folder = checkPath(folder.replace(/^\/+|\/+$/g, ""));
    if (!this.listing().folders.includes(folder)) fail(ErrorCode.notFound, `${folder} does not exist`);
    const bases = new Map<string, string>();
    for (const f of listed) {
      if (!f.path.startsWith(`${folder}/`)) fail(ErrorCode.invalidParams, `${f.path} is not in ${folder}`);
      bases.set(f.path, f.base);
    }
    const res: FolderDeleteResult = { deleted: [], kept: [], removed: false };
    const inside = [...this.files.keys()].filter((p) => p.startsWith(`${folder}/`));
    for (const p of [...new Set([...inside, ...bases.keys()])].sort(cmp)) {
      const e = this.files.get(p);
      const b = bases.get(p);
      if (!e) continue;
      if (b === undefined || b !== e.hash) res.kept.push(p);
      else {
        this.remove(e, who);
        res.deleted.push(p);
      }
    }
    if (res.kept.length === 0) {
      for (const f of [...this.folders]) if (f === folder || f.startsWith(`${folder}/`)) this.folders.delete(f);
      res.removed = true;
    } else {
      for (const f of [...this.folders]) {
        if (f.startsWith(`${folder}/`) && !res.kept.some((k) => k.startsWith(`${f}/`))) this.folders.delete(f);
      }
    }
    return res;
  }

  resolve(from: string, target: string): string | undefined {
    if (target === "") return from;
    const t = target.replace(/^\/+/, "");
    const candidates = kindOf(t) === "note" || t.includes(".") ? [t] : [t, `${t}.md`];
    const folder = dir(from);
    for (const c of candidates) {
      const rel = folder === "" ? c : `${folder}/${c}`;
      if (this.files.has(rel)) return rel;
    }
    for (const c of candidates) if (this.files.has(c)) return c;
    const matches = [...this.files.keys()].filter((p) =>
      candidates.some((c) => p.toLowerCase() === c.toLowerCase() || p.toLowerCase().endsWith(`/${c.toLowerCase()}`)),
    );
    matches.sort((a, b) => a.split("/").length - b.split("/").length || cmp(a, b));
    return matches[0];
  }

  resolutions(from: string, targets: string[]): Resolution[] {
    checkPath(from);
    if (targets.length > 1000) fail(ErrorCode.invalidParams, `${targets.length} targets: at most 1000`);
    return targets.map((target) => {
      const path = this.resolve(from, target);
      return path === undefined ? { target } : { target, path };
    });
  }

  /** render draws a note's text as a small subset of notesd's HTML (docs/specs/render.md). */
  render(p: string, content?: string): string {
    checkPath(p);
    if (kindOf(p) !== "note") fail(ErrorCode.invalidParams, `${p} is not a note: only notes render`);
    const text = content ?? this.read(p).content;
    const body = text.replace(/^---\n[\s\S]*?\n---\n?/, "");
    const out: string[] = [];
    for (const block of body.split(/\n{2,}/)) {
      if (block.trim() === "") continue;
      const h = /^(#{1,6}) (.*)$/.exec(block);
      if (h) {
        const level = (h[1] ?? "#").length;
        out.push(`<h${level}>${this.inline(p, h[2] ?? "")}</h${level}>`);
        continue;
      }
      const lines = block.split("\n").map((l) => this.inline(p, l));
      out.push(`<p>${lines.join("<br>\n")}</p>`);
    }
    return out.join("\n");
  }

  private inline(from: string, line: string): string {
    return escapeHTML(line)
      .replace(/\[\[([^\]|#]*)(#[^\]|]*)?(\|[^\]]*)?\]\]/g, (_, target: string, sub = "", alias = "") => {
        const path = this.resolve(from, target);
        const text = alias ? alias.slice(1) : target + (sub ? ` > ${sub.slice(1)}` : "");
        const cls = path === undefined ? "internal-link is-unresolved" : "internal-link";
        const dataPath = path === undefined ? "" : ` data-path="${escapeHTML(path)}"`;
        return `<a class="${cls}" data-href="${target}${sub}"${dataPath}>${text}</a>`;
      })
      .replace(/(^|\s)#([\p{L}\p{N}_/-]+)/gu, '$1<a class="tag" data-tag="$2">#$2</a>');
  }

  template(p: string, values: { title: string; at: Date }): string {
    const content = this.read(p).content;
    const date = isoDate(values.at);
    const time = values.at.toISOString().slice(11, 16);
    return content
      .replace(/\{\{\s*title\s*\}\}/gi, values.title)
      .replace(/\{\{\s*date\s*\}\}/gi, date)
      .replace(/\{\{\s*time\s*\}\}/gi, time);
  }

  entries(): NoteEntry[] {
    return [...this.files.values()]
      .filter((e) => kindOf(e.path) === "note")
      .sort((a, b) => cmp(a.path, b.path))
      .map((e) => ({
        path: e.path,
        title: title(e.path),
        hash: e.hash,
        size: e.data.length,
        modifiedAt: e.modifiedAt,
      }));
  }

  search(query: string, limit = 20): SearchHit[] {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (words.length === 0) return [];
    const hits: SearchHit[] = [];
    for (const n of this.entries()) {
      const text = this.text(n.path) ?? "";
      const lower = `${n.title}\n${text}`.toLowerCase();
      if (!words.every((w) => lower.includes(w))) continue;
      const first = words[0] ?? "";
      const at = text.toLowerCase().indexOf(first);
      const snippet =
        at < 0
          ? [{ text: text.slice(0, 80), match: false }]
          : [
              { text: text.slice(Math.max(0, at - 30), at), match: false },
              { text: text.slice(at, at + first.length), match: true },
              { text: text.slice(at + first.length, at + first.length + 50), match: false },
            ];
      hits.push({ path: n.path, title: n.title, snippet });
      if (hits.length >= limit) break;
    }
    return hits;
  }

  links(p: string): Link[] {
    const e = this.entry(p);
    if (kindOf(p) !== "note") return [];
    const out: Link[] = [];
    decoder
      .decode(e.data)
      .split("\n")
      .forEach((line, i) => {
        for (const m of line.matchAll(/(!?)\[\[([^\]|#]*)(#[^\]|]*)?(\|[^\]]*)?\]\]/g)) {
          const link: Link = {
            kind: "wiki",
            target: m[2] ?? "",
            subpath: m[3] ?? "",
            alias: m[4]?.slice(1) ?? "",
            embed: m[1] === "!",
            key: "",
            line: i + 1,
          };
          const resolved = this.resolve(p, m[2] ?? "");
          if (resolved !== undefined) link.resolved = resolved;
          out.push(link);
        }
      });
    return out;
  }

  backlinks(p: string): { source: string; link: Link }[] {
    this.entry(p);
    const out: { source: string; link: Link }[] = [];
    for (const n of this.entries()) {
      for (const link of this.links(n.path)) if (link.resolved === p) out.push({ source: n.path, link });
    }
    return out;
  }

  tagCounts(): TagCount[] {
    const counts = new Map<string, TagCount>();
    for (const n of this.entries()) {
      for (const tag of new Set(this.read(n.path).tags.map((t) => t.toLowerCase()))) {
        const c = counts.get(tag) ?? { tag, count: 0 };
        c.count++;
        counts.set(tag, c);
      }
    }
    return [...counts.values()].sort((a, b) => cmp(a.tag, b.tag));
  }

  propertyCounts(): PropertyCount[] {
    const builtIn: Record<string, string> = { aliases: "aliases", cssclasses: "multitext", tags: "tags" };
    const counts = new Map<string, PropertyCount>();
    for (const n of this.entries()) {
      for (const key of Object.keys(frontmatter(this.text(n.path) ?? ""))) {
        const folded = key.toLowerCase();
        const c = counts.get(folded) ?? { key, type: builtIn[folded] ?? "", count: 0 };
        c.count++;
        counts.set(folded, c);
      }
    }
    return [...counts.entries()].sort(([a], [b]) => cmp(a, b)).map(([, c]) => c);
  }

  // --- the vault domain

  getSettings(): VaultSettings {
    return structuredClone(this.settings);
  }

  configure(changes: Partial<VaultSettings>): VaultSettings {
    const next = { ...this.settings, sources: { ...this.settings.sources } };
    for (const [k, v] of Object.entries(changes)) {
      if (v === undefined || k === "id" || !(k in DEFAULT_SETTINGS) || k === "sources") continue;
      (next as unknown as Record<string, unknown>)[k] = v;
      next.sources[k] = "frostnotes";
    }
    this.settings = next;
    this.env.emit({ event: "vault.configured", data: { id: this.id } });
    return this.getSettings();
  }

  counts(): { notes: number; attachments: number } {
    const notes = [...this.files.keys()].filter((p) => kindOf(p) === "note").length;
    return { notes, attachments: this.files.size - notes };
  }

  // --- changes made outside notesd, for tests and the demo

  /** outside makes a change as another program would, or sync with source "sync": content null deletes. */
  outside(p: string, content: string | null, source: "outside" | "sync" = "outside"): void {
    const who: Author = { source, name: "" };
    const e = this.files.get(p);
    if (content === null) {
      if (e) this.remove(e, who);
      return;
    }
    this.put(checkPath(p), encoder.encode(content), who, true);
  }

  // --- internals

  private entry(p: string): Entry {
    checkPath(p);
    const e = this.files.get(p);
    if (!e) return fail(ErrorCode.notFound, `${p} does not exist`);
    return e;
  }

  private addFolder(p: string): void {
    this.folders.add(p);
    for (const d of parents(p)) this.folders.add(d);
  }

  private versionsOf(fileId: number): StoredVersion[] {
    return this.versions.filter((v) => v.fileId === fileId).reverse();
  }

  // put writes data at p, keeping a version and emitting note.changed when
  // emit is set.
  private put(p: string, data: Uint8Array, who: Author, emit: boolean): Entry {
    const hash = sha256(data);
    const at = this.env.now().toISOString();
    let e = this.files.get(p);
    if (!e) {
      const last = this.versions.findLast((v) => v.path === p && v.deleted);
      e = { id: last?.fileId ?? this.env.nextId(), path: p, data, hash, modifiedAt: at };
      this.files.set(p, e);
    } else if (e.hash === hash) {
      return e;
    } else {
      Object.assign(e, { data, hash, modifiedAt: at });
    }
    this.keep(e, data, who, false);
    if (emit) this.env.emit({ event: "note.changed", data: { vaultId: this.id, path: p, hash, source: who.source } });
    return e;
  }

  private keep(e: Entry, data: Uint8Array, who: Author, conflict: boolean): StoredVersion {
    const hash = sha256(data);
    this.blobs.set(hash, data);
    const v: StoredVersion = {
      id: this.env.nextId(),
      fileId: e.id,
      path: e.path,
      hash,
      size: data.length,
      at: this.env.now().toISOString(),
      source: who.source,
      author: who.name,
      deleted: false,
      conflict,
    };
    this.versions.push(v);
    return v;
  }

  private remove(e: Entry, who: Author): void {
    this.files.delete(e.path);
    this.versions.push({
      id: this.env.nextId(),
      fileId: e.id,
      path: e.path,
      hash: "",
      size: 0,
      at: this.env.now().toISOString(),
      source: who.source,
      author: who.name,
      deleted: true,
      conflict: false,
    });
    this.env.emit({ event: "note.removed", data: { vaultId: this.id, path: e.path } });
  }
}

/** defaultMerge lets a side that did not change take the other's; else the newer side stands and the hunk conflicts. */
export function defaultMerge(m: MergeInput): MergeOutput {
  if (m.incoming === m.base || m.incoming === m.head) return { content: m.head, conflicts: 0 };
  if (m.head === m.base) return { content: m.incoming, conflicts: 0 };
  return { content: m.head, conflicts: 1 };
}

function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

function isoDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

// frontmatter reads a note's YAML frontmatter as flat keys: scalars as
// strings, [a, b] and "- item" lists as arrays. The mock needs no more.
function frontmatter(text: string): Record<string, unknown> {
  const m = /^---\n([\s\S]*?)\n---(\n|$)/.exec(text);
  if (!m) return {};
  const out: Record<string, unknown> = {};
  let key = "";
  for (const line of (m[1] ?? "").split("\n")) {
    const item = /^\s+-\s+(.*)$/.exec(line);
    if (item && key) {
      out[key] = [...list(out[key]), (item[1] ?? "").trim()];
      continue;
    }
    const kv = /^([^:\s][^:]*):\s*(.*)$/.exec(line);
    if (!kv) continue;
    key = (kv[1] ?? "").trim();
    const v = (kv[2] ?? "").trim();
    out[key] =
      v.startsWith("[") && v.endsWith("]")
        ? v
            .slice(1, -1)
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean)
        : v === ""
          ? []
          : v;
  }
  return out;
}

function list(v: unknown): string[] {
  if (Array.isArray(v)) return v.map(String);
  return typeof v === "string" && v !== "" ? [v] : [];
}

function inlineTags(text: string): string[] {
  const body = text.replace(/^---\n[\s\S]*?\n---\n?/, "").replace(/```[\s\S]*?```/g, "");
  return [...body.matchAll(/(^|\s)#([\p{L}\p{N}_/-]*[\p{L}_/-][\p{L}\p{N}_/-]*)/gu)].map((m) => m[2] ?? "");
}
