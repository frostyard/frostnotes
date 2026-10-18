// The folder chooser's results (docs/specs/ui.md, Files pane: Move file
// to…). Task T-0063 replaces this stub.
import type { FuzzyMatch } from "./fuzzy";

/** FolderResult is a row of the chooser: a folder's path ("" for the root) and its match. */
export interface FolderResult {
  path: string;
  /** Spans in the path as shown ("/" for the root); null with no query. */
  match: FuzzyMatch | null;
}

/** folderText is how the chooser shows a folder: its path, or "/" for the root. */
export function folderText(path: string): string {
  return path;
}

/** folderResults lists the rows for query over every folder of the vault (the root not among them). */
export function folderResults(_folders: readonly string[], _query: string): FolderResult[] {
  return [];
}
