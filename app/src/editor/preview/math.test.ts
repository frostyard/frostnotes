// @vitest-environment jsdom
// T-0073's given tests: math in live preview, drawn with KaTeX in an
// editor's view, in jsdom (KaTeX draws nothing in happy-dom, which it
// takes for a page in quirks mode).
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import katex from "katex";
import { afterEach, describe, expect, it, vi } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { blockDecorations, livePreview, previewEnv } from "./framework";
import { MATH } from "./math";

let view: EditorView | null = null;
afterEach(() => {
  vi.restoreAllMocks();
  view?.destroy();
  view = null;
});

function state(doc: string, cursor: number | string = doc.length): EditorState {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const s = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...MATH]),
      previewEnv.of({ resolve: () => null, command: () => {} }),
    ],
  });
  ensureSyntaxTree(s, s.doc.length, 10_000);
  return s;
}

function open(doc: string, cursor?: number | string): EditorView {
  view = new EditorView({ state: state(doc, cursor), parent: document.body });
  return view;
}

const maths = (v: EditorView) => [...v.contentDOM.querySelectorAll<HTMLElement>(".cm-pv-math")];
const press = (el: Element | undefined) => el?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));

/** replaced lists what the block decorations replace, by text. */
function replaced(s: EditorState): string[] {
  const out: string[] = [];
  for (const it = blockDecorations(s).iter(); it.value; it.next()) out.push(s.doc.sliceString(it.from, it.to));
  return out;
}

describe("inline math", () => {
  it("is drawn with KaTeX away from the selection's lines", () => {
    const v = open("Area $\\pi r^2$ here\n\nend");
    const m = maths(v);
    expect(m).toHaveLength(1);
    expect(m[0]?.tagName).toBe("SPAN");
    expect(m[0]?.dataset.pv).toBe("math");
    expect(m[0]?.querySelector(".katex")).not.toBeNull();
    expect(v.contentDOM.querySelector(".cm-line")?.textContent).not.toContain("$");
  });

  it("shows its source on the selection's lines", () => {
    const v = open("Area $\\pi r^2$ here\n\nend", "here");
    expect(maths(v)).toHaveLength(0);
    expect(v.contentDOM.querySelector(".cm-line")?.textContent).toBe("Area $\\pi r^2$ here");
  });

  it("shows a TeX error without throwing", () => {
    const v = open("Bad $\\frac{1$ math\n\nend");
    expect(maths(v)[0]?.querySelector(".katex-error")).not.toBeNull();
  });

  it("shows its TeX as written when KaTeX refuses to draw", () => {
    vi.spyOn(katex, "render").mockImplementation(() => {
      throw new Error("KaTeX doesn't work in quirks mode.");
    });
    expect(maths(open("Area $x^2$ here\n\nend"))[0]?.textContent).toBe("x^2");
  });

  it("puts the cursor in its source when clicked", () => {
    const doc = "Area $x^2$ here\n\nend";
    const v = open(doc);
    press(maths(v)[0]);
    expect(v.state.selection.main.head).toBe(doc.indexOf("x^2"));
  });
});

describe("block math", () => {
  const doc = "Before\n\n$$\ne = mc^2\n$$\n\nend";

  it("is a block of its own over the block's lines away from the selection", () => {
    expect(replaced(state(doc))).toEqual(["$$\ne = mc^2\n$$"]);
    const v = open(doc);
    const m = maths(v);
    expect(m).toHaveLength(1);
    expect(m[0]?.tagName).toBe("DIV");
    expect(m[0]?.classList.contains("cm-pv-math-block")).toBe(true);
    expect(m[0]?.querySelector(".katex-display")).not.toBeNull();
  });

  it("shows its source while the selection is in it", () => {
    expect(replaced(state(doc, "mc^2"))).toEqual([]);
  });

  it("draws a one-line block too", () => {
    expect(replaced(state("$$ a + b $$\n\nend"))).toEqual(["$$ a + b $$"]);
  });

  it("puts the cursor in its source when clicked", () => {
    const v = open(doc);
    press(maths(v)[0]);
    expect(v.state.selection.main.head).toBe(doc.indexOf("$$") + 2);
    expect(maths(v)).toHaveLength(0);
  });
});
