// A note's words and characters, as notesd counts its words
// (docs/specs/markdown.md#words), excluding frontmatter and comments.
import type { EditorState } from "@codemirror/state";

import { findComments } from "./syntax/comments";
import { frontmatterEnd } from "./syntax/extract";

/** Counts are the status bar's counts. */
export interface Counts {
  words: number;
  characters: number;
}

/** countWords counts the words and characters of the body between from and to (the whole note by default). */
export function countWords(state: EditorState, from = 0, to = state.doc.length): Counts {
  const text = state.doc.toString();
  let pos = Math.max(from, frontmatterEnd(text));
  const end = Math.min(to, text.length);
  const counts: Counts = { words: 0, characters: 0 };
  for (const comment of findComments(state)) {
    if (comment.to <= pos) continue;
    if (comment.from >= end) break;
    countText(text.slice(pos, Math.min(comment.from, end)), counts);
    pos = comment.to;
  }
  if (pos < end) countText(text.slice(pos, end), counts);
  return counts;
}

// Go's unicode.IsSpace differs from JavaScript's \s (notably U+0085 and U+FEFF).
const SPACE = /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]/u;
const LETTER_OR_DIGIT = /[\p{L}\p{Nd}]/u;

function countText(text: string, counts: Counts): void {
  let hasLetterOrDigit = false;
  for (const character of text) {
    if (character !== "\n" && character !== "\r") counts.characters++;
    if (SPACE.test(character)) {
      hasLetterOrDigit = false;
    } else if (!hasLetterOrDigit && LETTER_OR_DIGIT.test(character)) {
      counts.words++;
      hasLetterOrDigit = true;
    }
  }
}
