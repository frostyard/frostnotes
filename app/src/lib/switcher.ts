// The quick switcher's results (docs/specs/ui.md, Prompts): recent notes
// for a blank query, otherwise ranked fuzzy matches of paths and aliases.
import { type FuzzyMatch, matchPath, prepareFuzzySearch, sortByScore } from "./fuzzy";

/** SWITCHER_LIMIT is the most results the switcher lists. */
export const SWITCHER_LIMIT = 50;

/** SwitcherNote is a note the switcher can open. */
export interface SwitcherNote {
  /** The note's vault path, with ".md". */
  path: string;
  /** Its frontmatter aliases. */
  aliases: readonly string[];
}

/** SwitcherResult is a row of the switcher. */
export type SwitcherResult =
  /** A note; match's spans are in its path without ".md", null for a recent note with no query. */
  | { kind: "note"; path: string; match: FuzzyMatch | null }
  /** An alias of the note at path; match's spans are in the alias. */
  | { kind: "alias"; path: string; alias: string; match: FuzzyMatch };

/** stripMd is a note's path without its ".md" (any case). */
export function stripMd(path: string): string {
  return path.replace(/\.md$/i, "");
}

/** switcherResults lists the rows for query, given the recently opened paths, newest first. */
export function switcherResults(
  notes: readonly SwitcherNote[],
  query: string,
  recent: readonly string[],
): SwitcherResult[] {
  const q = query.trim();
  if (!q) {
    const remaining = new Set(notes.map((note) => note.path));
    const results: SwitcherResult[] = [];
    for (const path of recent) {
      if (!remaining.delete(path)) continue;
      results.push({ kind: "note", path, match: null });
      if (results.length === SWITCHER_LIMIT) break;
    }
    return results;
  }

  const search = prepareFuzzySearch(q);
  const results: (SwitcherResult & { match: FuzzyMatch })[] = [];
  for (const { path, aliases } of notes) {
    const match = matchPath(search, stripMd(path));
    if (match) results.push({ kind: "note", path, match });
    for (const alias of aliases) {
      const aliasMatch = search(alias);
      if (aliasMatch) results.push({ kind: "alias", path, alias, match: aliasMatch });
    }
  }
  return sortByScore(results).slice(0, SWITCHER_LIMIT);
}
