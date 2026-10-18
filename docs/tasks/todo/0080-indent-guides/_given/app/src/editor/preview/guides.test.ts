// T-0080's given tests: indentation guides down nested lists, drawn in an
// editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { livePreview, previewEnv } from "./framework";
import { GUIDES } from "./guides";

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

function open(doc: string): EditorView {
  const state = EditorState.create({
    doc,
    selection: EditorSelection.cursor(doc.length),
    extensions: [markdownSupport(), livePreview(GUIDES), previewEnv.of({ resolve: () => null, command: () => {} })],
  });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  view = new EditorView({ state, parent: document.body });
  return view;
}

/** guides lists, per line, the columns of its guides. */
function guides(v: EditorView): number[][] {
  return [...v.contentDOM.querySelectorAll(".cm-line")].map((line) =>
    [...line.querySelectorAll(".cm-pv-guide")].map((g) => {
      const pos = v.posAtDOM(g);
      return pos - v.state.doc.lineAt(pos).from;
    }),
  );
}

describe("indentation guides", () => {
  it("run down a nested list at each parent's bullet", () => {
    const v = open("- a\n  - b\n    - c\n  - d\n- e\n\nend");
    expect(guides(v)).toEqual([[], [0], [0, 2], [0], [], [], []]);
    expect([...v.contentDOM.querySelectorAll(".cm-pv-guide")].map((g) => g.textContent)).toEqual([" ", " ", " ", " "]);
  });

  it("keep the text as it is", () => {
    const doc = "- a\n  - b\n\nend";
    expect(open(doc).contentDOM.querySelectorAll(".cm-line")[1]?.textContent).toBe("  - b");
  });

  it("follow ordered lists, tabs and an item's continued lines", () => {
    expect(guides(open("1. a\n   continued\n   1. b\n\nend"))).toEqual([[], [0], [0], [], []]);
    expect(guides(open("- a\n\t- b\n\nend"))).toEqual([[], [0], [], []]);
  });

  it("are not drawn for an item without a nested list, or across blank lines", () => {
    expect(guides(open("- a\n- b\n\nend")).flat()).toEqual([]);
    expect(guides(open("- a\n\n  - b\n\nend"))).toEqual([[], [], [0], [], []]);
  });
});
