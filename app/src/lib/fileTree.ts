// The Files pane's rows (docs/specs/ui.md, Files pane): a vault's folders
// and files nested, sorted as Obsidian's file explorer sorts them, filtered
// by type, and flattened by which folders are open.

/** FileType is what Obsidian makes of a file, by its extension. */
export type FileType = "note" | "canvas" | "base" | "image" | "audio" | "video" | "pdf" | "other";

/** TreeFile is a file of the vault: its path and when it was last modified. */
export interface TreeFile {
  /** Vault-relative and slash-separated, as notesd gives it ("Projects/Garden plan.md"). */
  path: string;
  /** Milliseconds since the epoch. */
  modifiedAt: number;
}

/** SortOrder is the Files pane's sort order, by Obsidian's names. */
export type SortOrder = "alphabetical" | "alphabeticalReverse" | "byModifiedTime" | "byModifiedTimeReverse";

/** TreeOptions say how to build the rows. */
export interface TreeOptions {
  sort: SortOrder;
  /** The open folders, by path. */
  expanded: ReadonlySet<string>;
  /** Show files of type "other" too (the Show unsupported files preference). */
  showUnsupported: boolean;
}

/** TreeRow is one row of the Files pane. */
export interface TreeRow {
  kind: "folder" | "file";
  /** The folder's or file's vault path. */
  path: string;
  /** A folder's name, or a file's name without its extension. */
  name: string;
  /** A file's extension in lower case, without the dot; "" for a folder or a file without one. */
  extension: string;
  /** A file's type; "folder" for a folder. */
  type: FileType | "folder";
  /** 0 for what sits at the vault's root. */
  depth: number;
  /** The folder is open; false for a file. */
  expanded: boolean;
}

/** extensionOf returns the extension of a path's last segment in lower case, without the dot, or "". */
export function extensionOf(path: string): string {
  const name = nameOf(path);
  const dot = name.lastIndexOf(".");
  return dot <= 0 ? "" : name.slice(dot + 1).toLowerCase();
}

const FILE_TYPES: Readonly<Record<string, FileType>> = {
  md: "note",
  canvas: "canvas",
  base: "base",
  pdf: "pdf",
  bmp: "image",
  png: "image",
  jpg: "image",
  jpeg: "image",
  gif: "image",
  svg: "image",
  webp: "image",
  avif: "image",
  mp3: "audio",
  wav: "audio",
  m4a: "audio",
  "3gp": "audio",
  flac: "audio",
  ogg: "audio",
  oga: "audio",
  opus: "audio",
  mp4: "video",
  webm: "video",
  ogv: "video",
  mov: "video",
  mkv: "video",
};

/** fileType returns what Obsidian makes of a file at path. */
export function fileType(path: string): FileType {
  const extension = extensionOf(path);
  return Object.hasOwn(FILE_TYPES, extension) ? (FILE_TYPES[extension] ?? "other") : "other";
}

/** ancestors returns the folders above path, outermost first: "a/b/c.md" gives ["a", "a/b"]. */
export function ancestors(path: string): string[] {
  const folders: string[] = [];
  for (let slash = path.indexOf("/"); slash !== -1; slash = path.indexOf("/", slash + 1)) {
    folders.push(path.slice(0, slash));
  }
  return folders;
}

const COLLATOR = new Intl.Collator(undefined, { usage: "sort", sensitivity: "base", numeric: true });

function nameOf(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

function parentOf(path: string): string {
  const slash = path.lastIndexOf("/");
  return slash === -1 ? "" : path.slice(0, slash);
}

function plainCompare(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

interface FileEntry {
  row: TreeRow;
  fullName: string;
  modifiedAt: number;
}

interface FolderEntry {
  path: string;
  name: string;
  folders: FolderEntry[];
  files: FileEntry[];
}

function compareFiles(a: FileEntry, b: FileEntry, sort: SortOrder): number {
  const alphabetical =
    COLLATOR.compare(a.row.name, b.row.name) ||
    COLLATOR.compare(a.fullName, b.fullName) ||
    plainCompare(a.fullName, b.fullName);
  switch (sort) {
    case "alphabetical":
      return alphabetical;
    case "alphabeticalReverse":
      return -alphabetical;
    case "byModifiedTime":
      return b.modifiedAt - a.modifiedAt || alphabetical;
    case "byModifiedTimeReverse":
      return a.modifiedAt - b.modifiedAt || alphabetical;
  }
}

function appendRows(folder: FolderEntry, depth: number, options: TreeOptions, rows: TreeRow[]): void {
  folder.folders.sort((a, b) => COLLATOR.compare(a.name, b.name) || plainCompare(a.name, b.name));
  for (const child of folder.folders) {
    const expanded = options.expanded.has(child.path);
    rows.push({
      kind: "folder",
      path: child.path,
      name: child.name,
      extension: "",
      type: "folder",
      depth,
      expanded,
    });
    if (expanded) appendRows(child, depth + 1, options, rows);
  }
  folder.files.sort((a, b) => compareFiles(a, b, options.sort));
  for (const file of folder.files) rows.push({ ...file.row, depth });
}

/**
 * treeRows returns the Files pane's rows for a vault's files and folders,
 * in display order. folders may name empty folders; folders that hold
 * files need not be listed.
 */
export function treeRows(files: readonly TreeFile[], folders: readonly string[], options: TreeOptions): TreeRow[] {
  const root: FolderEntry = { path: "", name: "", folders: [], files: [] };
  const byPath = new Map<string, FolderEntry>([["", root]]);
  function ensureFolder(path: string): FolderEntry {
    const existing = byPath.get(path);
    if (existing) return existing;
    const folder: FolderEntry = { path, name: nameOf(path), folders: [], files: [] };
    byPath.set(path, folder);
    ensureFolder(parentOf(path)).folders.push(folder);
    return folder;
  }

  for (const path of folders) ensureFolder(path);
  for (const file of files) {
    const parent = ensureFolder(parentOf(file.path));
    const type = fileType(file.path);
    if (type === "other" && !options.showUnsupported) continue;
    const fullName = nameOf(file.path);
    const extension = extensionOf(file.path);
    parent.files.push({
      fullName,
      modifiedAt: file.modifiedAt,
      row: {
        kind: "file",
        path: file.path,
        name: extension ? fullName.slice(0, fullName.lastIndexOf(".")) : fullName,
        extension,
        type,
        depth: 0,
        expanded: false,
      },
    });
  }
  const rows: TreeRow[] = [];
  appendRows(root, 0, options, rows);
  return rows;
}
