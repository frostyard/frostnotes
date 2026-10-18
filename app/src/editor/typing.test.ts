// The Editor settings in a view: line numbers, folding, smart lists,
// Markdown's marks around a selection and the indentation guides' switch.
import { foldable } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView, runScopeHandlers } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { type EditorPrefs, editorExtensions } from "./extensions";
import { indentFoldRange } from "./typing";

const PREFS: EditorPrefs = {
  live: true,
  readable: true,
  fontSize: 16,
  spellcheck: true,
  autoPairBrackets: true,
  autoPairMarkdown: true,
  smartLists: true,
  indentWithTabs: true,
  tabSize: 4,
  lineNumbers: false,
  foldHeadings: true,
  foldIndent: true,
  indentGuides: true,
};

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

function open(doc: string, over: Partial<EditorPrefs> = {}, selection = EditorSelection.cursor(doc.length)) {
  view = new EditorView({
    parent: document.body,
    state: EditorState.create({
      doc,
      selection,
      extensions: editorExtensions({
        prefs: { ...PREFS, ...over },
        env: { resolve: () => null, command: () => {} },
        onBlur: () => {},
      }),
    }),
  });
  return view;
}

const enter = (v: EditorView) =>
  runScopeHandlers(v, new KeyboardEvent("keydown", { key: "Enter", bubbles: true }), "editor");

describe("the Editor settings", () => {
  it("show line numbers when asked", () => {
    expect(open("a\nb").dom.querySelector(".cm-lineNumbers")).toBeNull();
    expect(open("a\nb", { lineNumbers: true }).dom.querySelector(".cm-lineNumbers")).not.toBeNull();
  });

  it("fold headings and indentation, with a gutter, while either is on", () => {
    const doc = "# Plan\ntext\n\n- a\n  - b\n  - c\n- d\n";
    const v = open(doc);
    expect(v.dom.querySelector(".cm-foldGutter")).not.toBeNull();
    expect(foldable(v.state, 0, 6)).toEqual({ from: 6, to: v.state.doc.length - 1 });
    const a = v.state.doc.line(4);
    expect(foldable(v.state, a.from, a.to)).toEqual({ from: a.to, to: v.state.doc.line(6).to });
    const off = open(doc, { foldHeadings: false, foldIndent: false });
    expect(off.dom.querySelector(".cm-foldGutter")).toBeNull();
  });

  it("continue a list on Enter with smart lists, and not without", () => {
    const v = open("- one");
    expect(enter(v)).toBe(true);
    expect(v.state.doc.toString()).toBe("- one\n- ");
    const w = open("- one", { smartLists: false });
    enter(w);
    expect(w.state.doc.toString()).not.toBe("- one\n- ");
  });

  it("wrap a selection in a Markdown mark typed over it", () => {
    const v = open("say hi", {}, EditorSelection.range(4, 6));
    const handled = v.state.facet(EditorView.inputHandler).some((h) => h(v, 4, 6, "*", () => v.state.update({})));
    expect(handled).toBe(true);
    expect(v.state.doc.toString()).toBe("say *hi*");
    expect(v.state.sliceDoc(v.state.selection.main.from, v.state.selection.main.to)).toBe("hi");
    const w = open("say hi", { autoPairMarkdown: false }, EditorSelection.range(4, 6));
    expect(w.state.facet(EditorView.inputHandler).some((h) => h(w, 4, 6, "*", () => w.state.update({})))).toBe(false);
  });

  it("switch the indentation guides off with a class", () => {
    expect(open("x", { indentGuides: false }).dom.classList.contains("cm-no-guides")).toBe(true);
    expect(open("x").dom.classList.contains("cm-no-guides")).toBe(false);
  });
});

describe("folding by indentation", () => {
  const state = (doc: string) => EditorState.create({ doc, extensions: EditorState.tabSize.of(4) });
  const at = (s: EditorState, n: number) => indentFoldRange(s, s.doc.line(n).from, s.doc.line(n).to);

  it("folds the deeper lines after a line, blank lines between them included", () => {
    const s = state("a\n  b\n\n  c\nd");
    expect(at(s, 1)).toEqual({ from: 1, to: s.doc.line(4).to });
  });

  it("folds nothing without deeper lines, nor at a blank line, and counts tabs", () => {
    expect(at(state("a\nb"), 1)).toBeNull();
    expect(at(state("a\n\n  b"), 2)).toBeNull();
    const s = state("\tx\n\t\ty\n  z");
    expect(at(s, 1)).toEqual({ from: 2, to: s.doc.line(2).to });
  });
});
