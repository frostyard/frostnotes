// Obsidian's %%comments%% (docs/specs/markdown.md#comments): ranges of the
// text, found as notesd finds them, not syntax nodes, since one may cross
// blocks. Task T-0067 replaces this stub.
import type { EditorState, Extension } from "@codemirror/state";

/** Span is a range of the document, from inclusive, to exclusive. */
export interface Span {
  from: number;
  to: number;
}

/** findComments lists the body's comments, each with its %% marks, in order. */
export function findComments(_state: EditorState): Span[] {
  return [];
}

/** commentStyle marks the comments in view with the cm-fn-comment class. */
export const commentStyle: Extension = [];
