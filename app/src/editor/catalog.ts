// What link and tag completion knows of the vault (editor/complete.ts;
// docs/design/app.md#extensions): the files store's files, link text by
// the vault's New link format, another note's headings and block IDs from
// its text (its open document's, else notesd's), and the vault's tags
// from note.tags, read again at most every 10 s, and the notes' aliases
// (data/aliases.ts).
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";

import { useAliases } from "../data/aliases";
import { byPath, useFiles } from "../data/files";
import { useVaultState } from "../data/vaultState";
import type { Client, VaultFile } from "../rpc/gen/api";
import type { CompletionCatalog, LinkFile } from "./complete";
import { headingsOf, headingsOfText } from "./outline";
import type { DocumentRegistry } from "./registry";
import { findBlocks } from "./syntax/extract";
import { markdownSupport } from "./syntax/index";

/** TAGS_TTL is how long the vault's tags are kept before they are read again, in ms. */
export const TAGS_TTL = 10_000;

const bare = (p: string) => p.replace(/\.md$/i, "");
const nameOf = (p: string) => bare(p.slice(p.lastIndexOf("/") + 1));

// These path operations and resolution mirror internal/markdown/resolve.go.
function cleanPath(path: string): string {
  const rooted = path.startsWith("/");
  const parts: string[] = [];
  for (const part of path.split("/")) {
    if (part === "" || part === ".") continue;
    if (part === ".." && parts.length > 0 && parts[parts.length - 1] !== "..") parts.pop();
    else if (part !== ".." || !rooted) parts.push(part);
  }
  return (rooted ? "/" : "") + parts.join("/") || ".";
}

function dirOf(path: string): string {
  return cleanPath(path.slice(0, path.lastIndexOf("/") + 1));
}

function matchesResolvePath(file: string, name: string, suffix: boolean): boolean {
  return file === name || (suffix && file.endsWith(`/${name}`));
}

function compareResolvePaths(a: string, b: string, name: string, folder: string, suffix: boolean): number {
  if (suffix) {
    const aLocal = dirOf(a) === folder;
    const bLocal = dirOf(b) === folder;
    if (aLocal !== bLocal) return aLocal ? -1 : 1;
  }
  const aExact = matchesResolvePath(a, name, suffix);
  const bExact = matchesResolvePath(b, name, suffix);
  if (aExact !== bExact) return aExact ? -1 : 1;
  if (suffix) {
    const depth = a.split("/").length - b.split("/").length;
    if (depth !== 0) return depth;
  }
  return a < b ? -1 : a > b ? 1 : 0;
}

function resolveName(name: string, folder: string, suffix: boolean, paths: readonly string[]): string {
  if (name === ".." || name.startsWith("../")) return "";
  const lower = name.toLowerCase();
  const base = lower.slice(lower.lastIndexOf("/") + 1);
  const matches = paths.filter(
    (file) =>
      file.slice(file.lastIndexOf("/") + 1).toLowerCase() === base &&
      matchesResolvePath(file.toLowerCase(), lower, suffix),
  );
  matches.sort((a, b) => compareResolvePaths(a, b, name, folder, suffix));
  return matches[0] ?? "";
}

function resolveLink(source: string, target: string, paths: readonly string[]): string {
  if (target === "") return source;
  const rooted = target.startsWith("/");
  target = target.replace(/^\/+/, "");
  const names = (target.toLowerCase().endsWith(".md") ? [target] : [`${target}.md`, target]).map(cleanPath);
  const folder = dirOf(source);
  for (const name of names) {
    const wanted = rooted ? name : cleanPath(`${folder}/${name}`);
    const match = resolveName(wanted, folder, false, paths);
    if (match !== "") return match;
  }
  if (rooted || target.startsWith("./") || target.startsWith("../")) return "";
  for (const name of names) {
    const match = resolveName(name, folder, true, paths);
    if (match !== "") return match;
  }
  return "";
}

/**
 * linkTextFor is what a link from the note at from writes to name path, by
 * the vault's link format: "absolute", the path; "relative", the path from
 * the note's folder; "shortest", the file's name when no other file has
 * it, else the path. A note's ".md" is left out. Text that would resolve to
 * another file from the linking note becomes the rooted path ("/" and the path).
 */
export function linkTextFor(path: string, from: string, paths: Iterable<string>, format: string): string {
  if (format === "relative") {
    const dir = from.split("/").slice(0, -1);
    const to = path.split("/");
    let i = 0;
    while (i < dir.length && i < to.length - 1 && dir[i] === to[i]) i++;
    return bare([...dir.slice(i).map(() => ".."), ...to.slice(i)].join("/"));
  }
  const files = [...paths];
  const checked = (text: string) => (resolveLink(from, text, files) === path ? text : `/${bare(path)}`);
  if (format === "absolute") return checked(bare(path));
  const name = nameOf(path).toLowerCase();
  let same = 0;
  for (const p of files) if (nameOf(p).toLowerCase() === name) same++;
  return checked(same > 1 ? bare(path) : nameOf(path));
}

/** CatalogSource is what a note's editor gives its catalog. */
export interface CatalogSource {
  vaultId: number;
  /** The edited note's path, which follows moves. */
  path: () => string;
  /** The edited note's state, while its view is there. */
  state: () => EditorState | null;
  /** The client, which changes on a reconnect. */
  client: () => Client;
  docs: DocumentRegistry;
}

/** completionCatalog is a note's editor's catalog. */
export function completionCatalog(src: CatalogSource): CompletionCatalog {
  let listed: {
    from: ReadonlyMap<string, VaultFile>;
    aliases: ReadonlyMap<string, readonly string[]>;
    files: LinkFile[];
  } | null = null;
  let tags: { name: string; count: number }[] = [];
  let tagsAt = Number.NEGATIVE_INFINITY;

  // textOf is the text of the note target leads to from the edited note; null when it leads to none.
  const textOf = async (target: string): Promise<string | null> => {
    if (target === "") return src.state()?.doc.toString() ?? null;
    const c = src.client();
    const [r] = await c.note.resolve({ vaultId: src.vaultId, from: src.path(), targets: [target] });
    if (!r?.path?.endsWith(".md")) return null;
    const open = src.docs.find(src.vaultId, r.path);
    if (open?.isLoaded) return open.text.toString();
    return (await c.note.read({ vaultId: src.vaultId, path: r.path })).content;
  };

  const catalog: CompletionCatalog = {
    files: () => {
      const files = useFiles.getState().files;
      const aliases = useAliases.getState().byPath;
      if (listed?.from !== files || listed.aliases !== aliases) {
        listed = {
          from: files,
          aliases,
          files: [...files.keys()].sort(byPath).map((path) => ({ path, aliases: aliases.get(path) ?? [] })),
        };
      }
      return listed.files;
    },
    linkText: (path) =>
      linkTextFor(
        path,
        src.path(),
        useFiles.getState().files.keys(),
        useVaultState.getState().settings?.linkFormat ?? "shortest",
      ),
    headings: async (target) => {
      const own = target === "" ? src.state() : null;
      const text = own ? null : await textOf(target);
      const headings = own ? headingsOf(own) : text === null ? [] : headingsOfText(text);
      return headings.map(({ level, text }) => ({ level, text }));
    },
    blocks: async (target) => {
      const text = await textOf(target);
      if (text === null) return [];
      const state = EditorState.create({ doc: text, extensions: markdownSupport() });
      ensureSyntaxTree(state, state.doc.length, 200);
      return findBlocks(state).map((b) => ({ id: b.id, text: state.doc.line(b.line).text.trim() }));
    },
    tags: () => {
      if (performance.now() - tagsAt > TAGS_TTL) {
        tagsAt = performance.now();
        src
          .client()
          .note.tags({ vaultId: src.vaultId })
          .then((counts) => {
            tags = counts.map((t) => ({ name: t.tag, count: t.count }));
          })
          .catch(() => {
            tagsAt = Number.NEGATIVE_INFINITY;
          });
      }
      return tags;
    },
  };
  // The first "#" finds the tags read.
  catalog.tags();
  return catalog;
}
