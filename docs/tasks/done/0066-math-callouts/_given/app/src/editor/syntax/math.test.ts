// T-0066's given tests: math in the editor's parser, and callouts read from
// a quote's first line.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import type { SyntaxNode } from "@lezer/common";
import { describe, expect, it } from "vitest";

import { readCallout } from "./callouts";
import { findLinks } from "./extract";
import { markdownSupport } from "./index";

function parsed(doc: string): EditorState {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  return state;
}

/** nodes lists the text of every node named name. */
function nodes(doc: string, name: string): string[] {
  const state = parsed(doc);
  const out: string[] = [];
  syntaxTree(state).iterate({
    enter: (n) => {
      if (n.name === name) out.push(state.doc.sliceString(n.from, n.to));
    },
  });
  return out;
}

/** names lists every node's name, in order. */
function names(doc: string): string[] {
  const out: string[] = [];
  syntaxTree(parsed(doc)).iterate({ enter: (n) => void out.push(n.name) });
  return out;
}

describe("inline math", () => {
  it("is $…$ with its marks", () => {
    expect(nodes("Area $\\pi r^2$ here", "InlineMath")).toEqual(["$\\pi r^2$"]);
    expect(nodes("Area $\\pi r^2$ here", "InlineMathMark")).toEqual(["$", "$"]);
  });

  it("is $$…$$ within a line too", () => {
    expect(nodes("So $$e = mc^2$$ holds", "InlineMath")).toEqual(["$$e = mc^2$$"]);
    expect(nodes("So $$e = mc^2$$ holds", "InlineMathMark")).toEqual(["$$", "$$"]);
  });

  it("opens only before a character that is not a space, and closes only after one", () => {
    expect(nodes("$ x$ and $x $ and $5 and $6", "InlineMath")).toEqual([]);
    expect(nodes("$a$ $b c$", "InlineMath")).toEqual(["$a$", "$b c$"]);
  });

  it("skips escaped dollars, and is not opened by one", () => {
    expect(nodes("$a\\$b$ and \\$c$", "InlineMath")).toEqual(["$a\\$b$"]);
  });

  it("stays on one line", () => {
    expect(nodes("one $a\nb$ two", "InlineMath")).toEqual([]);
    expect(nodes("$$a\nb$$", "InlineMath")).toEqual([]);
  });

  it("hides links and other syntax", () => {
    const doc = "Inline $[[x]] *y*$ math, then [[Real]].";
    expect(findLinks(parsed(doc)).map((l) => l.target)).toEqual(["Real"]);
    expect(names(doc)).not.toContain("Emphasis");
  });

  it("is not math in code", () => {
    expect(nodes("`$x$`", "InlineMath")).toEqual([]);
  });
});

describe("block math", () => {
  it("runs from a line starting $$ to the next line holding $$", () => {
    const doc = "Before\n\n$$\n[[In math]]\nx = 1 *no*\n$$\nAfter [[Real]]\n";
    expect(nodes(doc, "BlockMath")).toEqual(["$$\n[[In math]]\nx = 1 *no*\n$$"]);
    expect(nodes(doc, "BlockMathMark")).toEqual(["$$", "$$"]);
    expect(findLinks(parsed(doc)).map((l) => l.target)).toEqual(["Real"]);
    expect(nodes(doc, "Paragraph")).toEqual(["Before", "After [[Real]]"]);
  });

  it("closes on its first line when that line holds $$ again", () => {
    expect(nodes("$$ x = 1 $$\ntext\n", "BlockMath")).toEqual(["$$ x = 1 $$"]);
    expect(nodes("$$x\ny$$ z\nafter\n", "BlockMath")).toEqual(["$$x\ny$$ z"]);
  });

  it("interrupts a paragraph", () => {
    expect(nodes("para\n$$\nx\n$$\n", "Paragraph")).toEqual(["para"]);
    expect(nodes("para\n$$\nx\n$$\n", "BlockMath")).toEqual(["$$\nx\n$$"]);
  });

  it("runs to the end when it never closes", () => {
    expect(nodes("$$\nx\n\ny [[z]]", "BlockMath")).toEqual(["$$\nx\n\ny [[z]]"]);
    expect(nodes("$$\nx\n\ny [[z]]", "WikiLink")).toEqual([]);
  });

  it("works in a quote, keeping the quote's marks", () => {
    const doc = "> $$\n> [[x]]\n> $$\n\nafter\n";
    expect(nodes(doc, "BlockMath")).toEqual(["$$\n> [[x]]\n> $$"]);
    expect(nodes(doc, "QuoteMark")).toHaveLength(3);
    expect(nodes(doc, "WikiLink")).toEqual([]);
  });

  it("ends with the quote it is in", () => {
    const doc = "> $$\n> x\n\nafter [[y]]\n";
    expect(nodes(doc, "BlockMath")).toEqual(["$$\n> x"]);
    expect(nodes(doc, "WikiLink")).toEqual(["[[y]]"]);
  });

  it("is code when indented four spaces", () => {
    expect(nodes("    $$\n    x\n    $$\n", "BlockMath")).toEqual([]);
  });
});

/** callouts reads every Blockquote of doc. */
function callouts(doc: string) {
  const state = parsed(doc);
  const out: ReturnType<typeof readCallout>[] = [];
  syntaxTree(state).iterate({
    enter: (n) => {
      if (n.name === "Blockquote") out.push(readCallout(state, n.node));
    },
  });
  return out;
}

describe("callouts", () => {
  it("read the type, fold and title of a quote's first line", () => {
    expect(callouts("> [!note] Watering\n> Check the soil.")).toEqual([
      { type: "note", fold: "", title: "Watering", markFrom: 2, markTo: 9 },
    ]);
    expect(callouts("> [!WARNING]- Folded  title \n> body")).toEqual([
      { type: "warning", fold: "-", title: "Folded  title", markFrom: 2, markTo: 13 },
    ]);
    expect(callouts(">[!my-type_2]+")).toEqual([{ type: "my-type_2", fold: "+", title: "", markFrom: 1, markTo: 14 }]);
  });

  it("are not plain quotes, or a marker later than the first line", () => {
    expect(callouts("> plain\n\n> [!bad type] x\n\n> [!note]title\n\n> text\n> [!note] later")).toEqual([
      null,
      null,
      null,
      null,
    ]);
  });

  it("are read in lists and nested quotes", () => {
    expect(callouts("- > [!tip] In a list")).toEqual([
      { type: "tip", fold: "", title: "In a list", markFrom: 4, markTo: 10 },
    ]);
    expect(callouts("> > [!info] Inner").map((c) => c?.type ?? null)).toEqual([null, "info"]);
  });

  it("are only quotes", () => {
    const state = parsed("[!note] Not a quote");
    const para = syntaxTree(state).topNode.firstChild as SyntaxNode;
    expect(para.name).toBe("Paragraph");
    expect(readCallout(state, para)).toBeNull();
  });
});
