// Obsidian's formatting commands (docs/specs/ui.md#keyboard-map), preserving
// selections while applying each command in one formatting transaction.
import {
  type ChangeSpec,
  EditorSelection,
  type EditorState,
  type Line,
  type SelectionRange,
  type StateCommand,
  type TransactionSpec,
} from "@codemirror/state";

function formatting(edit: (state: EditorState) => TransactionSpec): StateCommand {
  return ({ state, dispatch }) => {
    if (state.readOnly) return false;
    dispatch(state.update({ ...edit(state), userEvent: "input.format", scrollIntoView: true }));
    return true;
  };
}

function starRuns(before: string, after: string): [number, number] {
  return [/\*{1,3}$/.exec(before)?.[0].length ?? 0, /^\*{1,3}/.exec(after)?.[0].length ?? 0];
}

function hasMark(before: string, after: string, mark: string): boolean {
  if (mark === "*" || mark === "**") {
    const n = Math.min(...starRuns(before, after));
    return mark === "**" ? n >= 2 : n % 2 === 1;
  }
  return before.endsWith(mark) && after.startsWith(mark);
}

function toggleSpan(state: EditorState, range: SelectionRange, mark: string) {
  const { from, to } = range;
  const size = mark.length;
  const before = state.sliceDoc(Math.max(0, from - 3), from);
  const after = state.sliceDoc(to, Math.min(state.doc.length, to + 3));
  const text = state.sliceDoc(from, to);
  let edits: ChangeSpec;
  let start = from;
  let end = to;
  if (hasMark(before, after, mark)) {
    edits = [
      { from: from - size, to: from },
      { from: to, to: to + size },
    ];
    start -= size;
    end -= size;
  } else if (text.length > 2 * size && hasMark(text, text, mark)) {
    const [trailing, leading] = mark.startsWith("*") ? starRuns(text, text) : [size, size];
    // A selection made only of stars must still have disjoint removals.
    const left = Math.min(leading, Math.floor(text.length / 2));
    const right = Math.min(trailing, Math.floor(text.length / 2));
    // Remove the stars nearest the text, keeping any other emphasis intact.
    edits = [
      { from: from + left - size, to: from + left },
      { from: to - right, to: to - right + size },
    ];
    end -= 2 * size;
  } else {
    edits =
      from === to
        ? { from, insert: mark + mark }
        : [
            { from, insert: mark },
            { from: to, insert: mark },
          ];
    start += size;
    end += size;
  }
  return {
    changes: state.changes(edits),
    range: range.anchor <= range.head ? EditorSelection.range(start, end) : EditorSelection.range(end, start),
  };
}

function toggleMark(mark: string): StateCommand {
  return formatting((state) =>
    state.changeByRange((range) => {
      const before = state.sliceDoc(Math.max(0, range.from - 3), range.from);
      const after = state.sliceDoc(range.to, Math.min(state.doc.length, range.to + 3));
      const word = range.empty && !hasMark(before, after, mark) ? state.wordAt(range.head) : null;
      const result = toggleSpan(state, word ?? range, mark);
      if (word) result.range = EditorSelection.cursor(result.range.from + range.head - word.from);
      return result;
    }),
  );
}

function touchedLines(state: EditorState): Line[] {
  const numbers = new Set<number>();
  for (const range of state.selection.ranges) {
    const last = state.doc.lineAt(range.to).number;
    for (let n = state.doc.lineAt(range.from).number; n <= last; n++) numbers.add(n);
  }
  return [...numbers].sort((a, b) => a - b).map((n) => state.doc.line(n));
}

function editLines(edit: (line: Line) => ChangeSpec): StateCommand {
  return formatting((state) => {
    const changes = state.changes(touchedLines(state).map(edit));
    return { changes, selection: state.selection.map(changes, 1) };
  });
}

/** toggleBold toggles ** around the selection or the word at the cursor. */
export const toggleBold: StateCommand = toggleMark("**");
/** toggleItalics toggles * around the selection or the word at the cursor. */
export const toggleItalics: StateCommand = toggleMark("*");
/** toggleStrikethrough toggles ~~. */
export const toggleStrikethrough: StateCommand = toggleMark("~~");
/** toggleHighlight toggles ==. */
export const toggleHighlight: StateCommand = toggleMark("==");
/** toggleCode toggles `. */
export const toggleCode: StateCommand = toggleMark("`");
/** toggleComment toggles %% around the selection, or around the cursor's line. */
export const toggleComment: StateCommand = formatting((state) =>
  state.changeByRange((range) => {
    if (!range.empty) return toggleSpan(state, range, "%%");
    const line = state.doc.lineAt(range.head);
    const from = line.from + (line.text.match(/^\s*/)?.[0].length ?? 0);
    const span = EditorSelection.range(from, line.to);
    // An empty comment is also removable, even though it has no selected text.
    const result =
      state.sliceDoc(from, line.to) === "%%%%"
        ? { changes: state.changes({ from, to: line.to }), range: EditorSelection.cursor(from) }
        : toggleSpan(state, span, "%%");
    const head =
      from === line.to && range.head === from
        ? result.range.from
        : result.changes.mapPos(range.head, range.head <= from ? 1 : -1);
    return { changes: result.changes, range: EditorSelection.cursor(head) };
  }),
);
/** insertLink writes a markdown link around the selection: its text, or its URL when it is one. */
export const insertLink: StateCommand = formatting((state) =>
  state.changeByRange((range) => {
    const text = state.sliceDoc(range.from, range.to);
    const url = /^[a-z][a-z0-9+.-]*:\/\/\S+$/i.test(text);
    const insert = url ? `[](${text})` : `[${text}]()`;
    const head = range.from + (range.empty || url ? 1 : text.length + 3);
    return { changes: { from: range.from, to: range.to, insert }, range: EditorSelection.cursor(head) };
  }),
);
/** toggleChecklist checks or unchecks each touched line's task, makes a list item a task, and any other line a task item. */
export const toggleChecklist: StateCommand = editLines((line) => {
  const indent = line.text.match(/^\s*/)?.[0] ?? "";
  const item = /^(?:[-*+]|\d+[.)])(?:[ \t]+|$)/.exec(line.text.slice(indent.length));
  const from = line.from + indent.length;
  if (!item) return { from, insert: "- [ ] " };
  const content = from + item[0].length;
  const task = /^\[(.)\](?=\s|$)/.exec(line.text.slice(content - line.from));
  if (task) return { from: content + 1, to: content + 2, insert: task[1] === " " ? "x" : " " };
  return { from: content, insert: /\s$/.test(item[0]) ? "[ ] " : " [ ] " };
});
/** deleteParagraph deletes each line the selection touches. */
export const deleteParagraph: StateCommand = formatting((state) => {
  const edits: { from: number; to: number }[] = [];
  for (const line of touchedLines(state)) {
    const from = line.number === state.doc.lines ? Math.max(0, line.from - 1) : line.from;
    const to = line.number === state.doc.lines ? line.to : line.to + 1;
    const previous = edits.at(-1);
    if (previous && from <= previous.to) previous.to = to;
    else edits.push({ from, to });
  }
  const changes = state.changes(edits);
  return { changes, selection: state.selection.map(changes) };
});

/** setHeading makes each touched line a heading of level (1 to 6), or plain text (0). */
export function setHeading(level: number): StateCommand {
  return editLines((line) => {
    const heading = /^( {0,3})(#{1,6})( +|$)/.exec(line.text);
    if (heading?.[2]?.length === level) return [];
    const indent = heading?.[1] ?? line.text.match(/^\s*/)?.[0] ?? "";
    const from = line.from + indent.length;
    const to = heading ? line.from + heading[0].length : from;
    return { from, to, insert: level === 0 ? "" : `${"#".repeat(level)} ` };
  });
}
