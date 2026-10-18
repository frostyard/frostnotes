// A note's words and characters, as notesd counts its words
// (docs/specs/markdown.md#words). Task T-0067 replaces this stub.
import type { EditorState } from "@codemirror/state";

/** Counts are the status bar's counts. */
export interface Counts {
  words: number;
  characters: number;
}

/** countWords counts the words and characters of the body between from and to (the whole note by default). */
export function countWords(_state: EditorState, _from?: number, _to?: number): Counts {
  return { words: 0, characters: 0 };
}
