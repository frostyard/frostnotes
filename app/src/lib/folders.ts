// The folder chooser's results (docs/specs/ui.md, Files pane: Move file
// to…): unique folder paths and the root, ranked by fuzzy matches.
import { type FuzzyMatch, matchPath, prepareFuzzySearch, sortByScore } from "./fuzzy";

/** FolderResult is a row of the chooser: a folder's path ("" for the root) and its match. */
export interface FolderResult {
  path: string;
  /** Spans in the path as shown ("/" for the root); null with no query. */
  match: FuzzyMatch | null;
}

/** folderText is how the chooser shows a folder: its path, or "/" for the root. */
export function folderText(path: string): string {
  return path === "" ? "/" : path;
}

/** folderResults lists the rows for query over every folder of the vault (the root not among them). */
export function folderResults(folders: readonly string[], query: string): FolderResult[] {
  const paths = [...new Set(folders)].filter((path) => path !== "").sort((a, b) => a.localeCompare(b));
  const trimmed = query.trim();
  if (!trimmed) return ["", ...paths].map((path) => ({ path, match: null }));

  const search = prepareFuzzySearch(trimmed);
  const results: { path: string; match: FuzzyMatch }[] = [];
  const rootMatch = search("/");
  if (rootMatch) results.push({ path: "", match: rootMatch });
  for (const path of paths) {
    const match = matchPath(search, path);
    if (match) results.push({ path, match });
  }
  return sortByScore(results);
}
