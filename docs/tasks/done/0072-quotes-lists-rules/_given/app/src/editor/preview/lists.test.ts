// T-0072's given tests: quotes, lists, rules and done tasks in live
// preview, drawn in an editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { livePreview, previewEnv } from "./framework";
import { LISTS } from "./lists";

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

function open(doc: string, cursor: number | string = doc.length): EditorView {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const state = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...LISTS]),
      previewEnv.of({ resolve: () => null, command: () => {} }),
    ],
  });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  view = new EditorView({ state, parent: document.body });
  return view;
}

const lines = (v: EditorView) => [...v.contentDOM.querySelectorAll<HTMLElement>(".cm-line")];
const texts = (v: EditorView) => lines(v).map((l) => l.textContent);
const marked = (v: EditorView, cls: string) => lines(v).map((l) => l.classList.contains(cls));

describe("quotes", () => {
  it("draw their lines with a bar and hide their marks away from the selection's lines", () => {
    const v = open("> one\n> two\n\nend");
    expect(marked(v, "cm-pv-quote")).toEqual([true, true, false, false]);
    expect(texts(v)).toEqual(["one", "two", "", "end"]);
  });

  it("show their marks on the selection's lines, keeping the bar", () => {
    const v = open("> one\n> two\n\nend", "one");
    expect(texts(v)).toEqual(["> one", "two", "", "end"]);
    expect(marked(v, "cm-pv-quote")).toEqual([true, true, false, false]);
  });

  it("hide every mark of a nested quote", () => {
    expect(texts(open("> > deep\n>no space\n\nend"))).toEqual(["deep", "no space", "", "end"]);
  });
});

describe("lists", () => {
  it("draw a bullet list's marks as dots away from the selection's lines", () => {
    const v = open("- a\n* b\n+ c\n\n1. d\n\nend");
    expect(v.contentDOM.querySelectorAll(".cm-pv-bullet")).toHaveLength(3);
    expect(texts(v)).toEqual([" a", " b", " c", "", "1. d", "", "end"]);
  });

  it("show the mark on the selection's line", () => {
    const v = open("- a\n- b\n\nend", "b");
    expect(texts(v)).toEqual([" a", "- b", "", "end"]);
  });

  it("leave a task's mark to its checkbox", () => {
    const v = open("- [ ] task\n- item\n\nend");
    expect(v.contentDOM.querySelectorAll(".cm-pv-bullet")).toHaveLength(1);
    expect(v.contentDOM.querySelectorAll("input.cm-pv-task")).toHaveLength(1);
  });

  it("mute a done task's text, wherever the selection is", () => {
    const doc = "- [x] done it\n* [X] Done\n- [ ] open\n- [/] doing\n\nend";
    const done = (v: EditorView) => [...v.contentDOM.querySelectorAll(".cm-pv-task-done")].map((el) => el.textContent);
    expect(done(open(doc))).toEqual(["done it", "Done"]);
    expect(done(open(doc, "done it"))).toEqual(["done it", "Done"]);
  });
});

describe("rules", () => {
  it("draw a horizontal rule as a line away from the selection's lines", () => {
    const v = open("a\n\n---\n\n***\n\nend");
    expect(v.contentDOM.querySelectorAll(".cm-pv-hr")).toHaveLength(2);
    expect(marked(v, "cm-pv-hr-line")).toEqual([false, false, true, false, true, false, false]);
    expect(texts(v)[2]).toBe("");
  });

  it("show the rule on the selection's line", () => {
    const v = open("a\n\n---\n\nend", 4);
    expect(texts(v)[2]).toBe("---");
    expect(v.contentDOM.querySelectorAll(".cm-pv-hr")).toHaveLength(0);
  });

  it("leave a setext heading's underline alone", () => {
    expect(open("Title\n---\n\nend").contentDOM.querySelectorAll(".cm-pv-hr")).toHaveLength(0);
  });
});
