// The quick switcher's results (docs/specs/ui.md, Prompts). Task T-0051
// replaces this stub.
import type { FuzzyMatch } from "./fuzzy";

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
  return path;
}

/** switcherResults lists the rows for query, given the recently opened paths, newest first. */
export function switcherResults(
  _notes: readonly SwitcherNote[],
  _query: string,
  _recent: readonly string[],
): SwitcherResult[] {
  return [];
}
