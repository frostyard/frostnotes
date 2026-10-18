// What link and tag completion knows of the vault (editor/complete.ts;
// docs/design/app.md#extensions): the files store's files, link text by
// the vault's New link format, another note's headings and block IDs from
// its text (its open document's, else notesd's), and the vault's tags
// from note.tags, read again at most every 10 s. Notes' aliases wait for
// note listings that carry them.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";

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

/**
 * linkTextFor is what a link from the note at from writes to name path, by
 * the vault's link format: "absolute", the path; "relative", the path from
 * the note's folder; "shortest", the file's name when no other file has
 * it, else the path. A note's ".md" is left out.
 */
export function linkTextFor(path: string, from: string, paths: Iterable<string>, format: string): string {
  if (format === "absolute") return bare(path);
  if (format === "relative") {
    const dir = from.split("/").slice(0, -1);
    const to = path.split("/");
    let i = 0;
    while (i < dir.length && i < to.length - 1 && dir[i] === to[i]) i++;
    return bare([...dir.slice(i).map(() => ".."), ...to.slice(i)].join("/"));
  }
  const name = nameOf(path).toLowerCase();
  let same = 0;
  for (const p of paths) if (nameOf(p).toLowerCase() === name) same++;
  return same > 1 ? bare(path) : nameOf(path);
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
  let listed: { from: ReadonlyMap<string, VaultFile>; files: LinkFile[] } | null = null;
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
      if (listed?.from !== files) {
        listed = { from: files, files: [...files.keys()].sort(byPath).map((path) => ({ path, aliases: [] })) };
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
