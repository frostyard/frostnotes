// Files into a note (ADR-0023, docs/design/app.md#attachments): a pasted,
// dropped or attached file goes through note.upload to where the vault's
// Default location for new attachments says, under a name no file has, and
// the note gets an embed of it at the cursor, as Obsidian does.
import { EditorSelection } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";

import type { Client, VaultSettings } from "../rpc/gen/api";

/** MAX_ATTACH is the most bytes notesd takes in one note.upload. */
export const MAX_ATTACH = 10 * 1024 * 1024;

/**
 * attachmentFolder is where a note's attachments go, by the setting as
 * Obsidian writes it: "/" the vault root, "./" the note's folder, "./NAME"
 * a folder NAME beside the note, else a folder's path. "" is the root.
 */
export function attachmentFolder(setting: string, notePath: string): string {
  const noteFolder = notePath.includes("/") ? notePath.slice(0, notePath.lastIndexOf("/")) : "";
  const join = (a: string, b: string) => [a, b].filter((x) => x !== "").join("/");
  if (setting === "" || setting === "/") return "";
  if (setting === "./" || setting === ".") return noteFolder;
  if (setting.startsWith("./")) return join(noteFolder, setting.slice(2).replace(/\/+$/, ""));
  return setting.replace(/^\/+|\/+$/g, "");
}

/** freePath is folder/name, or name with " 1", " 2"… before its extension, the first no file has. */
export function freePath(folder: string, name: string, taken: (path: string) => boolean): string {
  const dot = name.lastIndexOf(".");
  const stem = dot > 0 ? name.slice(0, dot) : name;
  const ext = dot > 0 ? name.slice(dot) : "";
  const at = (n: string) => (folder === "" ? n : `${folder}/${n}`);
  if (!taken(at(name))) return at(name);
  for (let i = 1; ; i++) {
    const candidate = at(`${stem} ${i}${ext}`);
    if (!taken(candidate)) return candidate;
  }
}

/** pastedName is Obsidian's name for a pasted image: "Pasted image YYYYMMDDHHmmss.ext", in local time. */
export function pastedName(at: Date, ext: string): string {
  const p = (n: number) => String(n).padStart(2, "0");
  const stamp = `${at.getFullYear()}${p(at.getMonth() + 1)}${p(at.getDate())}${p(at.getHours())}${p(at.getMinutes())}${p(at.getSeconds())}`;
  return `Pasted image ${stamp}.${ext}`;
}

const MIME_EXT: Record<string, string> = {
  "image/png": "png",
  "image/jpeg": "jpg",
  "image/gif": "gif",
  "image/webp": "webp",
  "image/avif": "avif",
  "image/heic": "heic",
  "image/svg+xml": "svg",
};

/** fileName is the name a file is uploaded under: its own, or a pasted image's for a nameless one. */
export function fileName(file: { name: string; type: string }, at: Date): string {
  const name = file.name.replace(/[\\/:]/g, "-").trim();
  if (name !== "" && name !== "image.png") return name;
  return pastedName(at, MIME_EXT[file.type] ?? "png");
}

/**
 * embedFor is the embed a note gets for an attachment: ![[name]] with
 * wikilinks, the path when another file has its name; else a markdown
 * image with the path, encoded.
 */
export function embedFor(path: string, wikilinks: boolean, nameIsUnique: boolean): string {
  const name = path.slice(path.lastIndexOf("/") + 1);
  if (wikilinks) return `![[${nameIsUnique ? name : path}]]`;
  return `![](${path.split("/").map(encodeURIComponent).join("/")})`;
}

/** base64 encodes bytes, standard alphabet, padded. */
export function base64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

/** UploadContext is what an upload needs to know about the vault and the note. */
export interface UploadContext {
  client: Client;
  vaultId: number;
  notePath: string;
  settings: Pick<VaultSettings, "attachmentFolder" | "wikilinks"> | null;
  /** The paths of the vault's files now. */
  paths: () => Iterable<string>;
  now?: () => Date;
}

/**
 * uploadFiles uploads each file next to the note, as its settings say,
 * and returns the embeds to insert, one per line. A file over MAX_ATTACH
 * or a refused upload throws, naming the file.
 */
export async function uploadFiles(ctx: UploadContext, files: readonly File[]): Promise<string> {
  const folder = attachmentFolder(ctx.settings?.attachmentFolder ?? "/", ctx.notePath);
  const paths = new Set(ctx.paths());
  const embeds: string[] = [];
  for (const file of files) {
    if (file.size > MAX_ATTACH) {
      throw new Error(`${file.name} is over 10 MB: copy it into the vault's folder instead`);
    }
    const path = freePath(folder, fileName(file, (ctx.now ?? (() => new Date()))()), (p) => paths.has(p));
    const data = base64(new Uint8Array(await file.arrayBuffer()));
    await ctx.client.note.upload({ vaultId: ctx.vaultId, path, data });
    const name = path.slice(path.lastIndexOf("/") + 1);
    const unique = ![...paths].some((p) => p !== path && p.slice(p.lastIndexOf("/") + 1) === name);
    paths.add(path);
    embeds.push(embedFor(path, ctx.settings?.wikilinks ?? true, unique));
  }
  return embeds.join("\n");
}

/**
 * insertAt writes text into view at pos, the cursor after it; at the start
 * of a line that holds text, the text gets a line of its own.
 */
export function insertAt(view: EditorView, pos: number, text: string): void {
  const line = view.state.doc.lineAt(pos);
  const own = pos === line.from && line.length > 0 ? "\n" : "";
  view.dispatch({
    changes: { from: pos, insert: text + own },
    selection: EditorSelection.cursor(pos + text.length),
    userEvent: "input.paste",
    scrollIntoView: true,
  });
}

/** pickFiles asks the browser for files to attach; empty when the person cancels. */
export function pickFiles(): Promise<File[]> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.multiple = true;
    input.addEventListener("change", () => resolve([...(input.files ?? [])]));
    input.addEventListener("cancel", () => resolve([]));
    input.click();
  });
}
