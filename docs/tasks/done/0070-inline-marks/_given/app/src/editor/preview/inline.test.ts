// T-0070's given tests: highlights, tags, inline code and footnote
// references in live preview, drawn in an editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { livePreview, type PreviewCommand, previewEnv } from "./framework";
import { INLINE } from "./inline";

let view: EditorView | null = null;
let commands: PreviewCommand[] = [];
afterEach(() => {
  view?.destroy();
  view = null;
  commands = [];
});

/** open shows doc with the cursor at cursor (a string the doc contains, or an offset). */
function open(doc: string, cursor: number | string = doc.length): EditorView {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const state = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...INLINE]),
      previewEnv.of({ resolve: () => "x.md", command: (c) => void commands.push(c) }),
    ],
  });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  view = new EditorView({ state, parent: document.body });
  return view;
}

const line = (v: EditorView, n: number) => v.contentDOM.querySelectorAll(".cm-line")[n]?.textContent;
const all = (v: EditorView, sel: string) => [...v.contentDOM.querySelectorAll<HTMLElement>(sel)];
const press = (el: Element | undefined) => el?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));

describe("highlights", () => {
  it("hide their marks away from the selection's lines", () => {
    expect(line(open("a ==b c== d\n\nend"), 0)).toBe("a b c d");
  });

  it("show their marks on the selection's lines", () => {
    expect(line(open("a ==b c== d\n\nend", "b c"), 0)).toBe("a ==b c== d");
  });
});

describe("tags", () => {
  it("are pills that name their tag, away from the selection's lines", () => {
    const v = open("x #garden/fruit y\n\nend");
    expect(all(v, ".cm-pv-tag").map((el) => [el.textContent, el.dataset.tag])).toEqual([
      ["#garden/fruit", "garden/fruit"],
    ]);
    expect(all(open("x #garden/fruit y\n\nend", "y"), ".cm-pv-tag")).toHaveLength(0);
  });

  it("are not pills in a link's text", () => {
    expect(all(open("[see #x](Plan.md) and #y\n\nend"), ".cm-pv-tag").map((el) => el.textContent)).toEqual(["#y"]);
  });

  it("search for themselves when clicked", () => {
    const v = open("x #garden/fruit y\n\nend");
    press(all(v, ".cm-pv-tag")[0]);
    expect(commands).toEqual([{ kind: "search", query: "tag:#garden/fruit" }]);
  });
});

describe("inline code", () => {
  it("hides its backticks and is drawn as code, away from the selection's lines", () => {
    const v = open("use `npm i` and ``a ` b``\n\nend");
    expect(all(v, ".cm-pv-inline-code").map((el) => el.textContent)).toEqual(["npm i", "a ` b"]);
    expect(line(v, 0)).toBe("use npm i and a ` b");
  });

  it("shows its backticks on the selection's lines", () => {
    const v = open("use `npm i` here\n\nend", "here");
    expect(line(v, 0)).toBe("use `npm i` here");
    expect(all(v, ".cm-pv-inline-code")).toHaveLength(0);
  });
});

describe("footnote references", () => {
  const doc = "Text[^1] more[^gone]\n\n[^1]: The note.\n\nend";

  it("are raised labels away from the selection's lines", () => {
    const v = open(doc);
    expect(all(v, "sup.cm-pv-footnote").map((el) => el.textContent)).toEqual(["1", "gone"]);
    expect(line(v, 0)).toBe("Text1 moregone");
    expect(line(open(doc, "more"), 0)).toBe("Text[^1] more[^gone]");
  });

  it("go to their definition when clicked, else into their source", () => {
    const v = open(doc);
    press(all(v, "sup.cm-pv-footnote")[0]);
    expect(v.state.selection.main.head).toBe(doc.indexOf("[^1]:"));
    const w = open(doc);
    press(all(w, "sup.cm-pv-footnote")[1]);
    expect(w.state.selection.main.head).toBe(doc.indexOf("[^gone]") + 2);
  });
});
