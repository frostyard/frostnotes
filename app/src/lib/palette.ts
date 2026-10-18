// The command palette's results (docs/specs/ui.md, Prompts). Task T-0052
// replaces this stub.
import type { FuzzyMatch } from "./fuzzy";

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
  _commands: readonly PaletteCommand[],
  _query: string,
  _recent: readonly string[],
): PaletteResult[] {
  return [];
}
