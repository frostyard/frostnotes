// T-0071's given tests: fenced code blocks in live preview, drawn in an
// editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { CODE } from "./code";
import { livePreview, type PreviewCommand, previewEnv, shownText } from "./framework";

let view: EditorView | null = null;
let commands: PreviewCommand[] = [];
afterEach(() => {
  view?.destroy();
  view = null;
  commands = [];
});

function open(doc: string, cursor: number | string = doc.length): EditorView {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const state = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...CODE]),
      previewEnv.of({ resolve: () => null, command: (c) => void commands.push(c) }),
    ],
  });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  view = new EditorView({ state, parent: document.body });
  return view;
}

const lines = (v: EditorView) => [...v.contentDOM.querySelectorAll<HTMLElement>(".cm-line")];
const shown = (v: EditorView, n: number) => {
  const el = lines(v)[n];
  return el ? shownText(el) : undefined;
};
const classes = (v: EditorView) =>
  lines(v).map((l) =>
    [...l.classList]
      .filter((c) => c.startsWith("cm-pv-code"))
      .sort()
      .join(" "),
  );

const DOC = "```js\nlet a = 1;\nlet b = 2;\n```\n\nend";

describe("code blocks", () => {
  it("draw every line as code, the first and the last marked", () => {
    expect(classes(open(DOC))).toEqual([
      "cm-pv-code cm-pv-code-first",
      "cm-pv-code",
      "cm-pv-code",
      "cm-pv-code cm-pv-code-last",
      "",
      "",
    ]);
    expect(classes(open(DOC, "a = 1"))[0]).toBe("cm-pv-code cm-pv-code-first");
  });

  it("hide their fences and show the language away from the selection", () => {
    const v = open(DOC);
    expect(v.contentDOM.querySelector(".cm-pv-code-lang")?.textContent).toBe("js");
    expect(shown(v, 0)).not.toContain("```");
    expect(shown(v, 3)).toBe("");
    expect(shown(v, 1)).toBe("let a = 1;");
  });

  it("show their fences while the selection is in them", () => {
    const v = open(DOC, "b = 2");
    expect(shown(v, 0)).toBe("```js");
    expect(shown(v, 3)).toBe("```");
    expect(v.contentDOM.querySelector(".cm-pv-code-header")).toBeNull();
  });

  it("show no language when the fence names none", () => {
    const v = open("~~~\nplain\n~~~\n\nend");
    expect(v.contentDOM.querySelector(".cm-pv-code-lang")?.textContent).toBe("");
    expect(shown(v, 2)).toBe("");
  });

  it("copy their code with the header's button", () => {
    const v = open(DOC);
    const copy = v.contentDOM.querySelector<HTMLButtonElement>("button.cm-pv-code-copy");
    expect(copy?.getAttribute("aria-label")).toBe("Copy code");
    copy?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
    expect(commands).toEqual([{ kind: "copy", text: "let a = 1;\nlet b = 2;" }]);
  });

  it("copy nothing from an empty block", () => {
    const v = open("```sh\n```\n\nend");
    v.contentDOM
      .querySelector("button.cm-pv-code-copy")
      ?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
    expect(commands).toEqual([{ kind: "copy", text: "" }]);
  });
});
