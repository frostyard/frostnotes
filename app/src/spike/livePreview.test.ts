import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { describe, expect, it } from "vitest";

import { markdownSupport, noteExtensions } from "./editor";
import { generateNote } from "./generate";
import { buildDecorations, toggleTask } from "./livePreview";

/** state parses doc fully, with the cursor at cursor (a string the doc contains, or an offset). */
function state(doc: string, cursor: number | string = doc.length) {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const s = EditorState.create({ doc, extensions: markdownSupport(), selection: EditorSelection.cursor(anchor) });
  ensureSyntaxTree(s, s.doc.length, 10_000);
  return s;
}

interface Drawn {
  kind: string;
  text: string;
}

/** drawn lists live preview's decorations over the whole doc, with the text each covers. */
function drawn(s: EditorState): Drawn[] {
  const out: Drawn[] = [];
  for (const it = buildDecorations(s, [{ from: 0, to: s.doc.length }]).iter(); it.value; it.next()) {
    out.push({ kind: String(it.value.spec.preview), text: s.doc.sliceString(it.from, it.to) });
  }
  return out;
}

const hidden = (s: EditorState) =>
  drawn(s)
    .filter((d) => d.kind === "hide")
    .map((d) => d.text);

describe("live preview", () => {
  it("hides emphasis, strikethrough and heading marks away from the cursor", () => {
    const doc = "## Title ##\nSome *em*, **strong** and ~~gone~~ text.\n\ncursor here";
    expect(hidden(state(doc))).toEqual(["## ", " ##", "*", "*", "**", "**", "~~", "~~"]);
    expect(drawn(state(doc)).filter((d) => d.kind === "heading")).toEqual([{ kind: "heading", text: "" }]);
  });

  it("shows the source on every line the selection touches", () => {
    const doc = "# One\nSome *em* text\n# Three";
    expect(hidden(state(doc, "em*"))).toEqual(["# ", "# "]);
    const both = EditorState.create({
      doc,
      extensions: markdownSupport(),
      selection: EditorSelection.single(2, doc.indexOf("Three")),
    });
    ensureSyntaxTree(both, both.doc.length, 10_000);
    expect(hidden(both)).toEqual([]);
  });

  it("shows a link's text and a wikilink's target or alias", () => {
    const doc = "See [the site](https://example.test/a), [[Garden Plan]] and [[Recipes#Soup|soup]].\n\nx";
    const d = drawn(state(doc));
    expect(d.filter((x) => x.kind === "hide").map((x) => x.text)).toEqual([
      "[",
      "](https://example.test/a)",
      "[[",
      "]]",
      "[[Recipes#Soup|",
      "]]",
    ]);
    expect(d.filter((x) => x.kind === "link").map((x) => x.text)).toEqual(["the site"]);
    expect(d.filter((x) => x.kind === "wikilink").map((x) => x.text)).toEqual(["Garden Plan", "soup"]);
    expect(drawn(state(doc, "Garden"))).toEqual([]);
  });

  it("leaves wikilinks and emphasis in code alone", () => {
    const doc = "`[[not]]` and\n\n```\n[[nor]] *this*\n```\n\nx";
    expect(drawn(state(doc)).filter((d) => d.kind !== "code")).toEqual([]);
    expect(drawn(state(doc)).filter((d) => d.kind === "code")).toHaveLength(3);
  });

  it("draws a checkbox per task unless the selection is on its marker", () => {
    const doc = "- [ ] open *task*\n- [x] done\n\nx";
    const d = drawn(state(doc)).filter((x) => x.kind === "checkbox");
    expect(d.map((x) => x.text)).toEqual(["- [ ]", "- [x]"]);
    expect(drawn(state(doc, 2)).filter((x) => x.kind === "checkbox")).toHaveLength(1);
    // The rest of the line still previews: the cursor is on the marker only.
    expect(hidden(state(doc, 2))).toEqual([]);
    expect(hidden(state(doc))).toEqual(["*", "*"]);
  });

  it("toggles a task by editing the one character between its brackets", () => {
    const doc = "Intro\n\n- [ ] open\n  - [X] nested done\n1. [ ] numbered\n";
    const s = state(doc, 0);
    const toggle = (st: EditorState, at: string) => {
      const spec = toggleTask(st, st.doc.toString().indexOf(at));
      if (!spec) throw new Error(`no task at ${at}`);
      const next = st.update(spec).state;
      ensureSyntaxTree(next, next.doc.length, 10_000);
      return next;
    };
    const once = toggle(s, "- [ ] open");
    expect(once.doc.toString()).toBe(doc.replace("- [ ] open", "- [x] open"));
    expect(toggle(once, "- [x] open").doc.toString()).toBe(doc);
    expect(toggle(s, "- [X] nested").doc.toString()).toBe(doc.replace("[X]", "[ ]"));
    expect(toggle(s, "[ ] numbered").doc.toString()).toBe(doc.replace("1. [ ]", "1. [x]"));
    expect(toggleTask(s, 0)).toBeNull();
  });

  it("keeps the document exactly the text it was given", () => {
    const note = generateNote(400);
    const parent = document.createElement("div");
    document.body.append(parent);
    const view = new EditorView({ state: EditorState.create({ doc: note, extensions: noteExtensions() }), parent });
    try {
      expect(view.state.doc.toString()).toBe(note);
      view.dispatch({ selection: { anchor: note.length } });
      expect(view.state.doc.toString()).toBe(note);
    } finally {
      view.destroy();
      parent.remove();
    }
  });
});
