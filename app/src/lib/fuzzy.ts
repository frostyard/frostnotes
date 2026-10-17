// Obsidian's fuzzy search (docs/specs/ui.md, Prompts): the quick switcher,
// the command palette and link suggestions rank what they offer with it.
// Task T-0039 implements it; until then nothing matches.

/** Range is a matched span of a text, [start, end) in UTF-16 code units. */
export type Range = [number, number];

/** FuzzyMatch is how well a query matched a text (higher is better, 0 at most) and where. */
export interface FuzzyMatch {
  score: number;
  /** The matched spans, in order, none touching another. */
  matches: Range[];
}

/** FuzzySearch matches one prepared query against a text; null when it does not match. */
export type FuzzySearch = (text: string) => FuzzyMatch | null;

/** prepareFuzzySearch prepares query once, for matching against many texts. */
export function prepareFuzzySearch(_query: string): FuzzySearch {
  return () => null;
}

/**
 * matchPath matches a vault path as the quick switcher does: its last
 * segment first, with the matches moved to where that segment starts in
 * path, else the whole path, for one point less.
 */
export function matchPath(_search: FuzzySearch, _path: string): FuzzyMatch | null {
  return null;
}

/** sortByScore returns items best first, keeping the order of equal scores. */
export function sortByScore<T extends { match: FuzzyMatch }>(items: readonly T[]): T[] {
  return [...items];
}
