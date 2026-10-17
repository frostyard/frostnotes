// Obsidian's fuzzy search (docs/specs/ui.md, Prompts): the quick switcher,
// the command palette and link suggestions rank what they offer with it.
// Matches tokens before falling back to characters, favoring compact matches
// at word starts and returning their merged spans in the original text.

const SPACE = /\s/;
const PUNCTUATION = /[ -⁯⸀-⹿\\'!"#$%&()*+,\-./:;<=>?@[\]^_`{|}~]/;
const CJK = /[ༀ-࿿぀-ヿ㐀-䶿一-鿿豈-﫿ｦ-ﾟ]/;

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
export function prepareFuzzySearch(query: string): FuzzySearch {
  const lowerQuery = query.toLowerCase();
  const tokens: string[] = [];
  const characters: string[] = [];
  let token = "";
  for (const c of lowerQuery.split("")) {
    if (c !== " ") characters.push(c);
    if (SPACE.test(c) || PUNCTUATION.test(c) || CJK.test(c)) {
      if (token) tokens.push(token);
      token = "";
      if (!SPACE.test(c)) tokens.push(c);
    } else {
      token += c;
    }
  }
  if (token) tokens.push(token);

  return (text) => {
    if (query === "") return { score: 0, matches: [] };
    const lower = text.toLowerCase();
    return (
      matchParts(tokens, text, lower, query.length, false) ?? matchParts(characters, text, lower, query.length, true)
    );
  };
}

function insideWord(text: string, at: number): boolean {
  if (at === 0) return false;
  const c = text.charAt(at);
  const p = text.charAt(at - 1);
  if (PUNCTUATION.test(c) || CJK.test(c)) return false;
  return (
    (c.toLowerCase() !== c && p.toLowerCase() !== p) ||
    (c.toUpperCase() !== c && !PUNCTUATION.test(p) && !SPACE.test(p) && !CJK.test(p))
  );
}

function matchParts(
  parts: string[],
  text: string,
  lower: string,
  queryLength: number,
  single: boolean,
): FuzzyMatch | null {
  const matches: Range[] = [];
  let position = 0;
  let count = 0;
  for (const part of parts) {
    const at = lower.indexOf(part, position);
    if (at === -1) return null;
    if ((!single || at !== position) && insideWord(text, at)) count++;
    const end = at + part.length;
    const last = matches.at(-1);
    if (last && last[1] >= at) {
      last[1] = end;
    } else {
      matches.push([at, end]);
    }
    position = end;
  }
  const first = matches[0];
  const last = matches.at(-1);
  if (!first || !last) return null;
  const score =
    -Math.max(0, matches.length - 1) -
    count / 10 -
    (last[1] - first[0] + 1 - queryLength) / 100 -
    first[0] / 1000 -
    lower.length / 10000;
  return { score, matches };
}

/**
 * matchPath matches a vault path as the quick switcher does: its last
 * segment first, with the matches moved to where that segment starts in
 * path, else the whole path, for one point less.
 */
export function matchPath(search: FuzzySearch, path: string): FuzzyMatch | null {
  const offset = path.lastIndexOf("/") + 1;
  const segmentMatch = search(path.slice(offset));
  if (segmentMatch) {
    return {
      score: segmentMatch.score,
      matches: segmentMatch.matches.map(([start, end]) => [start + offset, end + offset]),
    };
  }
  const match = search(path);
  return match ? { score: match.score - 1, matches: match.matches } : null;
}

/** sortByScore returns items best first, keeping the order of equal scores. */
export function sortByScore<T extends { match: FuzzyMatch }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => b.match.score - a.match.score);
}
