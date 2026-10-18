// The command palette's results (docs/specs/ui.md, Prompts): recent commands
// lead an empty query; fuzzy name matches are ranked with ties by name.
import { type FuzzyMatch, prepareFuzzySearch, sortByScore } from "./fuzzy";

/** PALETTE_RECENT is how many recently run commands lead an empty query's list. */
export const PALETTE_RECENT = 10;

/** PaletteCommand is a command the palette offers. */
export interface PaletteCommand {
  id: string;
  name: string;
  /** Its keys as formatHotkey writes them ("Ctrl + O"), first binding first. */
  keys: readonly string[];
}

/** PaletteResult is a row of the palette; match is null with no query. */
export interface PaletteResult {
  command: PaletteCommand;
  match: FuzzyMatch | null;
}

/** paletteResults lists the rows for query, given the IDs of the commands run, newest first. */
export function paletteResults(
  commands: readonly PaletteCommand[],
  query: string,
  recent: readonly string[],
): PaletteResult[] {
  const sorted = [...commands].sort((a, b) => a.name.localeCompare(b.name));
  const q = query.trim();
  if (q) {
    const search = prepareFuzzySearch(q);
    const matches: { command: PaletteCommand; match: FuzzyMatch }[] = [];
    for (const command of sorted) {
      const match = search(command.name);
      if (match) matches.push({ command, match });
    }
    return sortByScore(matches);
  }

  const byId = new Map(sorted.map((command) => [command.id, command]));
  const seen = new Set<string>();
  const results: PaletteResult[] = [];
  for (const id of recent) {
    if (seen.size === PALETTE_RECENT) break;
    const command = byId.get(id);
    if (!command || seen.has(id)) continue;
    seen.add(id);
    results.push({ command, match: null });
  }
  for (const command of sorted) {
    if (!seen.has(command.id)) results.push({ command, match: null });
  }
  return results;
}
