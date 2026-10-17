// The Files pane's rows (docs/specs/ui.md, Files pane): a vault's folders
// and files nested, sorted as Obsidian's file explorer sorts them, filtered
// by type, and flattened by which folders are open. Task T-0040 implements
// it; until then the tree is empty.

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
export function extensionOf(_path: string): string {
  return "";
}

/** fileType returns what Obsidian makes of a file at path. */
export function fileType(_path: string): FileType {
  return "other";
}

/** ancestors returns the folders above path, outermost first: "a/b/c.md" gives ["a", "a/b"]. */
export function ancestors(_path: string): string[] {
  return [];
}

/**
 * treeRows returns the Files pane's rows for a vault's files and folders,
 * in display order. folders may name empty folders; folders that hold
 * files need not be listed.
 */
export function treeRows(_files: readonly TreeFile[], _folders: readonly string[], _options: TreeOptions): TreeRow[] {
  return [];
}
