// Link and tag completion (docs/design/app.md#extensions). Task T-0069
// replaces this stub.
import type { CompletionSource } from "@codemirror/autocomplete";

/** COMPLETION_LIMIT is the most options a completion lists. */
export const COMPLETION_LIMIT = 50;

/** LinkFile is a file a link may name. */
export interface LinkFile {
  /** Its vault path. */
  path: string;
  /** A note's aliases; [] for other files. */
  aliases: readonly string[];
}

/** CompletionCatalog is what completion knows of the vault. */
export interface CompletionCatalog {
  /** Every file, by path. */
  files(): readonly LinkFile[];
  /** What a link to path writes as its target, by the vault's link format: "Plan", "Orchard/Plan", "map.png". */
  linkText(path: string): string;
  /** The headings of the note a target leads to ("" is the note being edited); [] when it leads nowhere. */
  headings(target: string): Promise<readonly { level: number; text: string }[]>;
  /** The block IDs of the note a target leads to, each with its line's text. */
  blocks(target: string): Promise<readonly { id: string; text: string }[]>;
  /** The vault's tags, without "#", with how many notes have each. */
  tags(): readonly { name: string; count: number }[];
}

/** linkCompletion completes a wikilink's target, heading or block ID. */
export function linkCompletion(_catalog: CompletionCatalog): CompletionSource {
  return () => null;
}

/** tagCompletion completes a tag after its "#". */
export function tagCompletion(_catalog: CompletionCatalog): CompletionSource {
  return () => null;
}
