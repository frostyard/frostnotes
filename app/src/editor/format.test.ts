// T-0068's given tests: the formatting commands. In a test's text, "|" is a
// cursor and "{…}" a selection.
import { EditorSelection, EditorState, type StateCommand } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import {
  deleteParagraph,
  insertLink,
  insertPair,
  setHeading,
  toggleBlockquote,
  toggleBold,
  toggleBulletList,
  toggleChecklist,
  toggleCode,
  toggleComment,
  toggleHighlight,
  toggleItalics,
  toggleNumberedList,
  toggleStrikethrough,
} from "./format";

/** at reads a test's text: its document and selection. */
function at(text: string, readOnly = false): EditorState {
  let doc = "";
  const ranges = [];
  let open = -1;
  for (const ch of text) {
    if (ch === "|") ranges.push(EditorSelection.cursor(doc.length));
    else if (ch === "{") open = doc.length;
    else if (ch === "}") ranges.push(EditorSelection.range(open, doc.length));
    else doc += ch;
  }
  return EditorState.create({
    doc,
    selection: EditorSelection.create(ranges.length > 0 ? ranges : [EditorSelection.cursor(0)]),
    extensions: [EditorState.allowMultipleSelections.of(true), EditorState.readOnly.of(readOnly)],
  });
}

/** show writes a state as a test's text. */
function show(state: EditorState): string {
  const doc = state.doc.toString();
  const marks: [number, string][] = [];
  for (const r of state.selection.ranges) {
    if (r.empty) marks.push([r.head, "|"]);
    else marks.push([r.from, "{"], [r.to, "}"]);
  }
  marks.sort((a, b) => a[0] - b[0] || (a[1] === "}" ? -1 : 1));
  let out = "";
  let pos = 0;
  for (const [p, m] of marks) {
    out += doc.slice(pos, p) + m;
    pos = p;
  }
  return out + doc.slice(pos);
}

/** run runs cmd on text and shows the result. */
function run(cmd: StateCommand, text: string): string {
  let state = at(text);
  const ran = cmd({
    state,
    dispatch: (tr) => {
      state = tr.state;
    },
  });
  expect(ran).toBe(true);
  return show(state);
}

describe("toggle bold", () => {
  it("wraps and unwraps a selection", () => {
    expect(run(toggleBold, "a {b c} d")).toBe("a **{b c}** d");
    expect(run(toggleBold, "a **{b c}** d")).toBe("a {b c} d");
    expect(run(toggleBold, "a {**b c**} d")).toBe("a {b c} d");
  });

  it("reads the stars of bold and italics together", () => {
    expect(run(toggleBold, "a ***{b}*** c")).toBe("a *{b}* c");
    expect(run(toggleBold, "a *{b}* c")).toBe("a ***{b}*** c");
  });

  it("applies to the word at the cursor, which keeps its place", () => {
    expect(run(toggleBold, "one wo|rd two")).toBe("one **wo|rd** two");
    expect(run(toggleBold, "one **wo|rd** two")).toBe("one wo|rd two");
    expect(run(toggleBold, "one word| two")).toBe("one **word|** two");
  });

  it("inserts a pair at a cursor outside a word, and takes an empty pair away", () => {
    expect(run(toggleBold, "a | b")).toBe("a **|** b");
    expect(run(toggleBold, "a **|** b")).toBe("a | b");
    expect(run(toggleBold, "|")).toBe("**|**");
  });

  it("applies to every range", () => {
    expect(run(toggleBold, "{a} and {b} and c|")).toBe("**{a}** and **{b}** and **c|**");
  });
});

describe("toggle italics", () => {
  it("wraps and unwraps with one star", () => {
    expect(run(toggleItalics, "a {b} c")).toBe("a *{b}* c");
    expect(run(toggleItalics, "a *{b}* c")).toBe("a {b} c");
    expect(run(toggleItalics, "a {*b*} c")).toBe("a {b} c");
    expect(run(toggleItalics, "x wo|rd")).toBe("x *wo|rd*");
  });

  it("tells italics from bold", () => {
    expect(run(toggleItalics, "a **{b}** c")).toBe("a ***{b}*** c");
    expect(run(toggleItalics, "a ***{b}*** c")).toBe("a **{b}** c");
    expect(run(toggleItalics, "**wo|rd**")).toBe("***wo|rd***");
  });
});

describe("the other marks", () => {
  it("toggle strikethrough", () => {
    expect(run(toggleStrikethrough, "{x}")).toBe("~~{x}~~");
    expect(run(toggleStrikethrough, "~~{x}~~")).toBe("{x}");
    expect(run(toggleStrikethrough, "a |")).toBe("a ~~|~~");
  });

  it("toggle highlight", () => {
    expect(run(toggleHighlight, "{x y}")).toBe("=={x y}==");
    expect(run(toggleHighlight, "==x|==")).toBe("x|");
  });

  it("toggle code", () => {
    expect(run(toggleCode, "use {npm}")).toBe("use `{npm}`");
    expect(run(toggleCode, "use `{npm}`")).toBe("use {npm}");
    expect(run(toggleCode, "use |")).toBe("use `|`");
  });
});

describe("toggle comment", () => {
  it("wraps and unwraps a selection", () => {
    expect(run(toggleComment, "a {b} c")).toBe("a %%{b}%% c");
    expect(run(toggleComment, "a %%{b}%% c")).toBe("a {b} c");
  });

  it("comments the cursor's line, after its indentation, and takes it back", () => {
    expect(run(toggleComment, "x\n  some |text\ny")).toBe("x\n  %%some |text%%\ny");
    expect(run(toggleComment, "x\n  %%some |text%%\ny")).toBe("x\n  some |text\ny");
    expect(run(toggleComment, "|")).toBe("%%|%%");
  });
});

describe("insert link", () => {
  it("makes a link of the selection's text, the cursor at its destination", () => {
    expect(run(insertLink, "see {the plan} now")).toBe("see [the plan](|) now");
  });

  it("makes a link of a selected URL, the cursor at its text", () => {
    expect(run(insertLink, "{https://example.com/x?y=1}")).toBe("[|](https://example.com/x?y=1)");
  });

  it("inserts an empty link at a cursor", () => {
    expect(run(insertLink, "a |b")).toBe("a [|]()b");
  });
});

describe("toggle checkbox status", () => {
  it("checks an open task, and opens any other", () => {
    expect(run(toggleChecklist, "- [ ] task|")).toBe("- [x] task|");
    expect(run(toggleChecklist, "* [x] done|")).toBe("* [ ] done|");
    expect(run(toggleChecklist, "  1. [/] doing|")).toBe("  1. [ ] doing|");
  });

  it("checks a task with nothing after its box", () => {
    expect(run(toggleChecklist, "- [ ]|")).toBe("- [x]|");
    expect(run(toggleChecklist, "- [x]|\n")).toBe("- [ ]|\n");
  });

  it("makes a list item a task", () => {
    expect(run(toggleChecklist, "- item|")).toBe("- [ ] item|");
    expect(run(toggleChecklist, "1) item|")).toBe("1) [ ] item|");
    expect(run(toggleChecklist, "-|")).toBe("- [ ] |");
  });

  it("makes any other line a task item, after its indentation", () => {
    expect(run(toggleChecklist, "|")).toBe("- [ ] |");
    expect(run(toggleChecklist, "  te|xt")).toBe("  - [ ] te|xt");
  });

  it("toggles every line the selection touches, once each, moving what was at an insertion past it", () => {
    expect(run(toggleChecklist, "{a\n- b\n- [ ] c}")).toBe("- [ ] {a\n- [ ] b\n- [x] c}");
    expect(run(toggleChecklist, "- [ ] a| b|")).toBe("- [x] a| b|");
  });
});

describe("delete paragraph", () => {
  it("deletes the cursor's line", () => {
    expect(run(deleteParagraph, "one\ntw|o\nthree")).toBe("one\n|three");
    expect(run(deleteParagraph, "one\ntwo\nthr|ee")).toBe("one\ntwo|");
    expect(run(deleteParagraph, "on|ly")).toBe("|");
  });

  it("deletes every line the selection touches", () => {
    expect(run(deleteParagraph, "o{ne\ntw}o\nthree")).toBe("|three");
    expect(run(deleteParagraph, "a|b|c\nd\ne|")).toBe("|d|");
  });
});

describe("set heading", () => {
  it("sets a line's level, and takes it away at 0", () => {
    expect(run(setHeading(2), "Title|")).toBe("## Title|");
    expect(run(setHeading(1), "### Title|")).toBe("# Title|");
    expect(run(setHeading(0), "## Title|")).toBe("Title|");
    expect(run(setHeading(0), "Plain|")).toBe("Plain|");
  });

  it("leaves a line at its level as it is", () => {
    expect(run(setHeading(2), "## Same|")).toBe("## Same|");
  });

  it("reads only a heading's marks", () => {
    expect(run(setHeading(1), "#tag text|")).toBe("# #tag text|");
    expect(run(setHeading(3), "#|")).toBe("### |");
  });

  it("sets every touched line, moving what was at an insertion past it", () => {
    expect(run(setHeading(3), "{a\n# b}")).toBe("### {a\n### b}");
    expect(run(setHeading(2), "|x")).toBe("## |x");
  });
});

describe("the phone toolbar's line commands", () => {
  it("quotes lines, and unquotes them when all are quoted", () => {
    expect(run(toggleBlockquote, "a|")).toBe("> a|");
    expect(run(toggleBlockquote, "{a\n\nb}")).toBe("> {a\n\n> b}");
    expect(run(toggleBlockquote, "{> a\n>b}")).toBe("{a\nb}");
  });

  it("makes bullets, numbered items from 1, and plain lines again", () => {
    expect(run(toggleBulletList, "{a\nb}")).toBe("- {a\n- b}");
    expect(run(toggleBulletList, "{- a\n- b}")).toBe("{a\nb}");
    expect(run(toggleBulletList, "1. a|")).toBe("- a|");
    expect(run(toggleNumberedList, "{a\nb\nc}")).toBe("1. {a\n2. b\n3. c}");
    expect(run(toggleNumberedList, "{1. a\n2. b}")).toBe("{a\nb}");
    expect(run(toggleNumberedList, "- a|")).toBe("1. a|");
    expect(run(toggleBulletList, "  a|")).toBe("  - a|");
  });

  it("inserts a wikilink, an embed or a tag around the selection", () => {
    expect(run(insertPair("[[", "]]"), "see |")).toBe("see [[|]]");
    expect(run(insertPair("[[", "]]"), "see {Plan}")).toBe("see [[Plan|]]");
    expect(run(insertPair("![[", "]]"), "|")).toBe("![[|]]");
    expect(run(insertPair("#", ""), "a |")).toBe("a #|");
  });
});

describe("a read-only editor", () => {
  it("runs no command", () => {
    const state = at("a {b} c", true);
    for (const cmd of [toggleBold, toggleComment, insertLink, toggleChecklist, deleteParagraph, setHeading(1)]) {
      expect(cmd({ state, dispatch: () => expect.fail("dispatched") })).toBe(false);
    }
  });
});
