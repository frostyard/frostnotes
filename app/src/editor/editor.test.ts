import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState, type Extension } from "@codemirror/state";
import { Decoration, EditorView, WidgetType } from "@codemirror/view";
import { describe, expect, it, vi } from "vitest";

import { editorKeymap, kept, keyInput } from "./keymap";
import { LinkResolver } from "./links";
import { BASIC } from "./preview/basic";
import {
  blockDecorations,
  buildInline,
  livePreview,
  type PreviewCommand,
  type Provider,
  previewEnv,
} from "./preview/framework";
import { markdownSupport } from "./syntax/index";

/** state parses doc fully, with the cursor at cursor (a string the doc contains, or an offset). */
function state(doc: string, cursor: number | string = doc.length, extra: Extension = []) {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const s = EditorState.create({
    doc,
    extensions: [markdownSupport(), livePreview(BASIC), extra],
    selection: EditorSelection.cursor(anchor),
  });
  ensureSyntaxTree(s, s.doc.length, 10_000);
  return s;
}

/** drawn lists the inline decorations over the whole doc: what each covers, and its class or widget. */
function drawn(s: EditorState): string[] {
  const out: string[] = [];
  for (const it = buildInline(s, [{ from: 0, to: s.doc.length }]).iter(); it.value; it.next()) {
    const spec = it.value.spec;
    const what = spec.widget ? "widget" : spec.class === "cm-pv-hide" ? "hide" : String(spec.class);
    out.push(`${what}:${s.doc.sliceString(it.from, it.to)}`);
  }
  return out;
}

describe("the editor's keymap", () => {
  it("leaves out what Obsidian removes and what the app's keymap binds", () => {
    expect(keyInput("Mod-Shift-z")).toEqual({ key: "z", ctrl: true, alt: false, shift: true, meta: false });
    expect(keyInput("Mod--")).toEqual({ key: "-", ctrl: true, alt: false, shift: false, meta: false });
    expect(kept({ key: "Alt-ArrowUp" })).toBe(false);
    expect(kept({ key: "Mod-i" })).toBe(false); // Toggle italics
    expect(kept({ key: "Mod-Enter" })).toBe(false); // Open link in new tab
    expect(kept({ key: "Mod-f" })).toBe(false); // Search current file
    expect(kept({ key: "Mod-z" })).toBe(true);
    const keys = editorKeymap().map((b) => b.key);
    expect(keys).toContain("Mod-z");
    expect(keys).toContain("Tab");
    expect(keys).not.toContain("Mod-d");
    expect(keys).not.toContain("Alt-l");
  });
});

describe("live preview's first providers", () => {
  it("hide marks and draw headings away from the selection, and show the source on its lines", () => {
    const doc = "## Title\nSome *em* and ~~gone~~.\n\nend";
    expect(drawn(state(doc))).toEqual(["cm-pv-h2:", "hide:## ", "hide:*", "hide:*", "hide:~~", "hide:~~"]);
    expect(drawn(state(doc, "em*"))).toEqual(["cm-pv-h2:", "hide:## "]);
  });

  it("draw links and wikilinks as links, dim the unresolved and read a subpath as Obsidian does", () => {
    const resolve = (t: string) => (t === "Plan" ? "Plan.md" : null);
    const env = previewEnv.of({ resolve, command: () => {} });
    const doc = "[[Plan#Goals]] [[Nowhere|alias]] [site](https://e.test) [[#Tasks]]\n\nend";
    expect(drawn(state(doc, doc.length, env))).toEqual([
      "hide:[[",
      "cm-pv-link:Plan",
      "widget:#",
      "cm-pv-link:Goals",
      "hide:]]",
      "hide:[[Nowhere|",
      "cm-pv-link cm-pv-unresolved:alias",
      "hide:]]",
      "hide:[",
      "cm-pv-link:site",
      "hide:](https://e.test)",
      "hide:[[",
      "widget:#",
      "cm-pv-link:Tasks",
      "hide:]]",
    ]);
  });

  it("draw a task's checkbox, which toggles one character when clicked", () => {
    const commands: PreviewCommand[] = [];
    const view = new EditorView({
      state: state(
        "- [ ] buy pears\n\nend",
        undefined,
        previewEnv.of({ resolve: () => null, command: (c) => void commands.push(c) }),
      ),
      parent: document.body,
    });
    const box = view.dom.querySelector<HTMLInputElement>("input.cm-pv-task");
    expect(box).not.toBeNull();
    box?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
    expect(view.state.doc.toString()).toBe("- [x] buy pears\n\nend");
    view.destroy();
  });

  it("send a link's click to the app as a command", () => {
    const commands: PreviewCommand[] = [];
    const view = new EditorView({
      state: state(
        "See [[Plan]].\n\nend",
        undefined,
        previewEnv.of({
          resolve: (t) => (t === "Plan" ? "Notes/Plan.md" : null),
          command: (c) => void commands.push(c),
        }),
      ),
      parent: document.body,
    });
    view.dom.querySelector(".cm-pv-link")?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, ctrlKey: true }));
    expect(commands).toEqual([{ kind: "follow", target: "Plan", path: "Notes/Plan.md", where: "tab" }]);
    view.destroy();
  });
});

describe("the parser", () => {
  it("reads a note's frontmatter as one node, none of it markdown", () => {
    const s = state("---\ntitle: [[Not a link]]\ntags: [a]\n---\n# Heading\n", 0);
    const names: string[] = [];
    syntaxTree(s)
      .topNode.cursor()
      .iterate((n) => void names.push(n.name));
    expect(names.slice(0, 3)).toEqual(["Document", "Frontmatter", "ATXHeading1"]);
    expect(state("---\nnot closed\n", 0).doc.length).toBeGreaterThan(0);
    const open = state("---\nnot closed\n\ntext\n", 0);
    expect(syntaxTree(open).topNode.firstChild?.name).toBe("HorizontalRule");
  });
});

describe("block providers", () => {
  class Box extends WidgetType {
    constructor(readonly text: string) {
      super();
    }
    override eq(o: Box) {
      return o.text === this.text;
    }
    toDOM() {
      return document.createElement("div");
    }
  }
  const draws = vi.fn();
  // A block provider that replaces a fenced code block, line breaks and all.
  const fence: Provider = {
    id: "fence",
    kind: "block",
    nodes: ["FencedCode"],
    draw(node, cx) {
      draws(node.from);
      if (!cx.touches(node.from, node.to)) {
        cx.add(
          Decoration.replace({ widget: new Box(cx.state.doc.sliceString(node.from, node.to)), block: true }).range(
            node.from,
            node.to,
          ),
        );
      }
      return false;
    },
  };
  const doc = "```\none\n```\n\ntext\n\n```\ntwo\n```\n";
  const blocks = (s: EditorState) => {
    const out: string[] = [];
    for (const it = blockDecorations(s).iter(); it.value; it.next()) out.push(s.doc.sliceString(it.from, it.to));
    return out;
  };

  it("replace whole blocks, show a block's source while the selection is in it, and redraw only around changes", () => {
    let s = state(doc, doc.indexOf("text"), livePreview([fence]));
    expect(blocks(s)).toEqual(["```\none\n```", "```\ntwo\n```"]);
    s = s.update({ selection: { anchor: doc.indexOf("two") } }).state;
    expect(blocks(s)).toEqual(["```\none\n```"]);
    draws.mockClear();
    s = s.update({ changes: { from: doc.indexOf("text"), insert: "more " } }).state;
    ensureSyntaxTree(s, s.doc.length, 10_000);
    expect(blocks(s)).toEqual(["```\none\n```"]);
    expect(draws.mock.calls.length).toBeLessThan(3); // not every block again
  });
});

describe("LinkResolver", () => {
  it("asks once for what is drawn together, and again after a reset", async () => {
    const ask = vi.fn(async (targets: string[]) =>
      targets.map((t) => (t === "A" ? { target: t, path: "A.md" } : { target: t })),
    );
    const changed = vi.fn();
    const r = new LinkResolver(ask, changed);
    expect([r.resolve("A"), r.resolve("B"), r.resolve("A")]).toEqual([undefined, undefined, undefined]);
    await vi.waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
    expect(ask).toHaveBeenCalledTimes(1);
    expect(ask).toHaveBeenCalledWith(["A", "B"]);
    expect([r.resolve("A"), r.resolve("B"), r.resolve("")]).toEqual(["A.md", null, null]);
    r.reset();
    expect(r.resolve("A")).toBeUndefined();
    await vi.waitFor(() => expect(ask).toHaveBeenCalledTimes(2));
  });
});
