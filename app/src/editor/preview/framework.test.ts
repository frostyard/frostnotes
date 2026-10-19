// The live-preview framework's drawing (docs/design/app.md#live-preview):
// hidden syntax is a mark, and the block providers' field draws what a
// parse adds as a full draw would.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { type DecorationSet, EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { blockDecorations, buildInline, livePreview, previewEnv, redraw, shownText } from "./framework";
import { MATH } from "./math";
import { RENDERED } from "./rendered";

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

function state(doc: string, cursor = doc.length): EditorState {
  return EditorState.create({
    doc,
    selection: EditorSelection.cursor(cursor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...MATH, ...RENDERED]),
      previewEnv.of({ resolve: () => null, command: () => {} }),
    ],
  });
}

describe("hidden syntax", () => {
  it("is a mark that does not show, so the line keeps its text and has no buffers", () => {
    const s = state("a **b** [x](https://e.test/) c\n\nend");
    ensureSyntaxTree(s, s.doc.length, 10_000);
    const hidden: string[] = [];
    for (const it = buildInline(s, [{ from: 0, to: s.doc.length }]).iter(); it.value; it.next()) {
      if (it.value.spec.class === "cm-pv-hide") hidden.push(s.doc.sliceString(it.from, it.to));
    }
    expect(hidden).toEqual(["**", "**", "[", "](https://e.test/)"]);
    view = new EditorView({ state: s, parent: document.body });
    const line = view.contentDOM.querySelector(".cm-line");
    expect(line?.textContent).toBe("a **b** [x](https://e.test/) c");
    expect(line && shownText(line)).toBe("a b x c");
    expect(line?.querySelectorAll("img.cm-widgetBuffer")).toHaveLength(0);
  });
});

// A block of each kind the field draws, then prose, many times over: more
// than the parse does when the state is made.
const BLOCKS = "$$\nx = 1\n$$\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n> [!note] Title\n> body\n\nSome *prose*.\n\n";
const LONG = BLOCKS.repeat(600);

// same compares two sets: their sizes, and where a's decoration is not b's.
function same(a: DecorationSet, b: DecorationSet): { a: number; b: number; differ: string[] } {
  const list = (set: DecorationSet) => {
    const out: { from: number; to: number; eq: (o: unknown) => boolean }[] = [];
    for (const it = set.iter(); it.value; it.next()) {
      const v = it.value;
      out.push({ from: it.from, to: it.to, eq: (o) => v.eq(o as typeof v) });
    }
    return out;
  };
  const xs = list(a);
  const ys: { from: number; to: number; value: unknown }[] = [];
  for (const it = b.iter(); it.value; it.next()) ys.push({ from: it.from, to: it.to, value: it.value });
  const differ: string[] = [];
  xs.forEach((x, i) => {
    const y = ys[i];
    if (!y || y.from !== x.from || y.to !== x.to || !x.eq(y.value)) differ.push(`${i}@${x.from}`);
  });
  return { a: xs.length, b: ys.length, differ };
}

describe("the block field", () => {
  it("draws what a parse adds as a full draw draws it", () => {
    const s0 = state(LONG);
    expect(syntaxTree(s0).length).toBeLessThan(LONG.length);
    ensureSyntaxTree(s0, LONG.length, 60_000);
    const s1 = s0.update({}).state;
    expect(syntaxTree(s1).length).toBe(LONG.length);
    const grown = blockDecorations(s1);
    const full = blockDecorations(s1.update({ effects: redraw.of(null) }).state);
    expect(same(grown, full)).toEqual({ a: 1800, b: 1800, differ: [] });
  });

  it("draws what a parse adds after an edit before where it stopped", () => {
    const s0 = state(LONG);
    const edited = s0.update({ changes: { from: 0, insert: "Intro\n\n" } }).state;
    ensureSyntaxTree(edited, edited.doc.length, 60_000);
    const s1 = edited.update({}).state;
    const full = blockDecorations(s1.update({ effects: redraw.of(null) }).state);
    expect(same(blockDecorations(s1), full)).toEqual({ a: 1800, b: 1800, differ: [] });
  });
});
