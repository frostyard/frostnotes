// The Files pane (docs/specs/ui.md, Files pane): its toolbar and the
// vault's tree, drawn from lib/fileTree.ts's rows. Task T-0059 replaces
// this stub.
import type { TreeRow } from "../../lib/fileTree";
import type { PaneClick } from "../panes/common";

/** DRAG_TYPE is the data type a dragged row carries: a JSON array of vault paths. */
export const DRAG_TYPE = "application/x-frostnotes-paths";

/** FilesCommand is what the pane asks of its container. */
export type FilesCommand =
  /** Open a file: a click, a middle click, or Enter. */
  | { kind: "open"; path: string; click: PaneClick }
  /** Open or close a folder. */
  | { kind: "toggle"; path: string }
  | { kind: "newNote" }
  | { kind: "newFolder" }
  /** Open the sort order's menu at (x, y). */
  | { kind: "sortMenu"; x: number; y: number }
  | { kind: "collapseAll" }
  | { kind: "expandAll" }
  /** Open the context menu of a row, or of the vault's root (path null), at (x, y). */
  | { kind: "menu"; path: string | null; folder: boolean; x: number; y: number }
  /** Start renaming a row (F2). */
  | { kind: "rename"; path: string }
  /** Rename a row to name, as typed (a file's name without its extension). */
  | { kind: "renamed"; path: string; name: string }
  | { kind: "cancelRename" }
  /** Move the dragged paths into folder ("" for the vault's root). */
  | { kind: "drop"; paths: string[]; folder: string };

export interface FilesPaneProps {
  rows: readonly TreeRow[];
  /** The active tab's file. */
  active: string | null;
  /** Some folder is open: the toolbar offers Collapse all, else Expand all. */
  anyExpanded: boolean;
  /** Draw the indentation guides. */
  indentGuides: boolean;
  /** The row whose name is being edited. */
  renaming: string | null;
  /** The listing has arrived: only then does an empty list mean an empty vault. */
  loaded: boolean;
  onCommand: (command: FilesCommand) => void;
}

/** FilesPane draws the Files pane. */
export function FilesPane(_props: FilesPaneProps) {
  return null;
}
