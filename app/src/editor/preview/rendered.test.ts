// T-0074's given tests: callouts, tables and note embeds in live preview,
// drawn as the environment renders them, in an editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { blockDecorations, livePreview, type PreviewCommand, previewEnv } from "./framework";
import { estimateHeight, RENDERED, RenderedWidget } from "./rendered";

let view: EditorView | null = null;
let commands: PreviewCommand[] = [];
let renders: string[] = [];
afterEach(() => {
  view?.destroy();
  view = null;
  commands = [];
  renders = [];
});

// RENDERED_HTML stands in for notesd's sanitized rendering of a block.
const RENDERED_HTML =
  '<div class="callout" data-callout="note" data-fold="-"><div class="callout-title">Title</div>' +
  '<div class="callout-content"><p><a class="internal-link" data-href="Plan#Goals" data-path="Plan.md">Plan</a> ' +
  '<a class="tag" data-tag="garden">#garden</a> <a class="external-link" href="https://e.test/">site</a> ' +
  "<span>text</span></p></div></div>";

const RESOLVE: Record<string, string | null> = { Plan: "Plan.md", "map.png": "media/map.png", Gone: null };

function state(doc: string, cursor: number | string = doc.length): EditorState {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const s = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...RENDERED]),
      previewEnv.of({
        resolve: (t) => RESOLVE[t],
        command: (c) => void commands.push(c),
        render: (el, markdown) => {
          renders.push(markdown);
          el.innerHTML = RENDERED_HTML;
        },
      }),
    ],
  });
  ensureSyntaxTree(s, s.doc.length, 10_000);
  return s;
}

function open(doc: string, cursor?: number | string): EditorView {
  view = new EditorView({ state: state(doc, cursor), parent: document.body });
  return view;
}

/** replaced lists what the block decorations replace, by text. */
function replaced(s: EditorState): string[] {
  const out: string[] = [];
  for (const it = blockDecorations(s).iter(); it.value; it.next()) out.push(s.doc.sliceString(it.from, it.to));
  return out;
}

const blocks = (v: EditorView) => [...v.contentDOM.querySelectorAll<HTMLElement>(".cm-pv-rendered")];
const press = (el: Element | null | undefined, init: MouseEventInit = {}) =>
  el?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0, ...init }));

const CALLOUT = "> [!note] Title\n> Body [[Plan]]\n\nend";

describe("callouts", () => {
  it("are blocks drawn as notesd renders their source, away from the selection", () => {
    expect(replaced(state(CALLOUT))).toEqual(["> [!note] Title\n> Body [[Plan]]"]);
    const v = open(CALLOUT);
    const b = blocks(v);
    expect(
      b.map((el) => [el.dataset.pv, el.classList.contains("rendered"), el.classList.contains("cm-pv-callout")]),
    ).toEqual([["callout", true, true]]);
    expect(renders).toEqual(["> [!note] Title\n> Body [[Plan]]"]);
  });

  it("show their source while the selection is in them", () => {
    expect(replaced(state(CALLOUT, "Body"))).toEqual([]);
  });

  it("leave plain quotes and quotes inside a line alone", () => {
    expect(replaced(state("> plain quote\n\n- > [!tip] in a list\n\nend"))).toEqual([]);
  });
});

describe("tables", () => {
  const doc = "| a | b |\n| - | - |\n| 1 | 2 |\n\nend";

  it("are blocks drawn as notesd renders them, away from the selection", () => {
    expect(replaced(state(doc))).toEqual(["| a | b |\n| - | - |\n| 1 | 2 |"]);
    expect(blocks(open(doc)).map((el) => [el.dataset.pv, el.classList.contains("cm-pv-table")])).toEqual([
      ["table", true],
    ]);
    expect(replaced(state(doc, "1 |"))).toEqual([]);
  });

  it("render again only when their source changes", () => {
    const v = open(doc);
    expect(renders).toHaveLength(1);
    v.dispatch({ changes: { from: v.state.doc.length, insert: " more" } });
    expect(renders).toHaveLength(1);
    v.dispatch({ changes: { from: doc.indexOf("2"), to: doc.indexOf("2") + 1, insert: "3" } });
    expect(renders).toEqual(["| a | b |\n| - | - |\n| 1 | 2 |", "| a | b |\n| - | - |\n| 1 | 3 |"]);
  });
});

describe("note embeds", () => {
  it("are blocks when alone on their line and leading to a note", () => {
    const doc = "![[Plan#Goals]]\n\nText ![[Plan]] inline\n\n![[map.png]]\n\n![[Gone]]\n\n- ![[Plan]]\n\nend";
    expect(replaced(state(doc))).toEqual(["![[Plan#Goals]]"]);
    expect(blocks(open(doc)).map((el) => [el.dataset.pv, el.classList.contains("cm-pv-embed")])).toEqual([
      ["embed", true],
    ]);
    expect(replaced(state(doc, "Goals"))).toEqual([]);
  });
});

describe("a click in a rendered block", () => {
  it("follows an internal link, in a new tab with Ctrl", () => {
    const v = open(CALLOUT);
    press(v.contentDOM.querySelector("a.internal-link"));
    press(v.contentDOM.querySelector("a.internal-link"), { ctrlKey: true });
    expect(commands).toEqual([
      { kind: "follow", target: "Plan", path: "Plan.md", where: "here" },
      { kind: "follow", target: "Plan", path: "Plan.md", where: "tab" },
    ]);
  });

  it("follows a web link and searches for a tag", () => {
    const v = open(CALLOUT);
    press(v.contentDOM.querySelector("a.external-link"));
    press(v.contentDOM.querySelector("a.tag"));
    expect(commands).toEqual([
      { kind: "follow", target: "https://e.test/", path: null, where: "here" },
      { kind: "search", query: "tag:#garden" },
    ]);
  });

  it("folds and unfolds a foldable callout from its title, the source staying hidden", () => {
    const v = open(CALLOUT);
    const callout = v.contentDOM.querySelector<HTMLElement>(".callout");
    press(v.contentDOM.querySelector(".callout-title"));
    expect(callout?.dataset.fold).toBe("+");
    press(v.contentDOM.querySelector(".callout-title"));
    expect(callout?.dataset.fold).toBe("-");
    expect(replaced(v.state)).toHaveLength(1);
    expect(commands).toEqual([]);
  });

  it("shows the source anywhere else", () => {
    const v = open(CALLOUT);
    press(v.contentDOM.querySelector(".callout-content span"));
    expect(v.state.selection.main.head).toBe(0);
    expect(replaced(v.state)).toEqual([]);
  });
});

describe("height estimates", () => {
  it("take a block for its source's lines, each wrapped, at the default size", () => {
    expect(estimateHeight("> [!note] Title\n> body")).toBe(48);
    expect(estimateHeight("x".repeat(160))).toBe(72);
    expect(new RenderedWidget("table", "| a |\n| - |\n| 1 |", "cm-pv-table").estimatedHeight).toBe(80);
  });
});
