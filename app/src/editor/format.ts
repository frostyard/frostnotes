// Obsidian's formatting commands (docs/specs/ui.md#keyboard-map). Task
// T-0068 replaces these stubs.
import type { StateCommand } from "@codemirror/state";

const stub: StateCommand = () => false;

/** toggleBold toggles ** around the selection or the word at the cursor. */
export const toggleBold: StateCommand = stub;
/** toggleItalics toggles * around the selection or the word at the cursor. */
export const toggleItalics: StateCommand = stub;
/** toggleStrikethrough toggles ~~. */
export const toggleStrikethrough: StateCommand = stub;
/** toggleHighlight toggles ==. */
export const toggleHighlight: StateCommand = stub;
/** toggleCode toggles `. */
export const toggleCode: StateCommand = stub;
/** toggleComment toggles %% around the selection, or around the cursor's line. */
export const toggleComment: StateCommand = stub;
/** insertLink writes a markdown link around the selection: its text, or its URL when it is one. */
export const insertLink: StateCommand = stub;
/** toggleChecklist checks or unchecks each touched line's task, makes a list item a task, and any other line a task item. */
export const toggleChecklist: StateCommand = stub;
/** deleteParagraph deletes each line the selection touches. */
export const deleteParagraph: StateCommand = stub;

/** setHeading makes each touched line a heading of level (1 to 6), or plain text (0). */
export function setHeading(_level: number): StateCommand {
  return stub;
}
