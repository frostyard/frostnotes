// T-0064's given tests: tags and highlights in the editor's parser.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import { findTags } from "./extract";
import { markdownSupport } from "./index";

function parsed(doc: string): EditorState {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  return state;
}

const tags = (doc: string) => findTags(parsed(doc)).map((t) => t.name);

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

/** inside lists the names of the nodes directly inside the first node named name. */
function inside(doc: string, name: string): string[] {
  const state = parsed(doc);
  const out: string[] = [];
  syntaxTree(state).iterate({
    enter: (n) => {
      if (n.name !== name || out.length > 0) return;
      for (let c = n.node.firstChild; c; c = c.nextSibling) out.push(c.name);
      return false;
    },
  });
  return out;
}

describe("tags", () => {
  it("start after white space or at the start of a block's text", () => {
    expect(tags("#a b #c\td\n#e and\u00a0#f")).toEqual(["a", "c", "e", "f"]);
  });

  it("count in headings, list items, quotes and table cells", () => {
    const doc = "# #h title\n\n- item #li\n\n>#quote\n\n| #cell | x |\n| - | - |\n|#c2|y|\n";
    expect(tags(doc)).toEqual(["h", "li", "quote", "cell", "c2"]);
  });

  it("take letters with their marks, digits, '_', '-' and '/'", () => {
    expect(tags("#café_notes #e\u0301te #area/work-life #日本語 #a1 #𝒜b")).toEqual([
      "café_notes",
      "e\u0301te",
      "area/work-life",
      "日本語",
      "a1",
      "𝒜b",
    ]);
  });

  it("end at the first other character", () => {
    expect(tags("#tag. #two,three #x)y #a#b #c🙂d #é!")).toEqual(["tag", "two", "x", "a", "c", "é"]);
  });

  it("are never all digits", () => {
    expect(tags("#123 #2026-10 #1a #_ #٣٤")).toEqual(["2026-10", "1a", "_"]);
  });

  it("need a name", () => {
    expect(tags("# Heading\n\n#\n\n## \n\na # b #🙂 #!")).toEqual([]);
  });

  it("are not tags after other characters, escaped, in code, links or URLs", () => {
    const doc =
      "a#b (#paren) \\#esc `#code` [[Note #nope]] [text #intext](Plan.md) ![alt #img](x.png) <https://x.y/#frag> http://e.com/#f\n";
    expect(tags(doc)).toEqual([]);
    expect(tags("```\n#fenced\n```\n\n    #indented\n")).toEqual([]);
  });

  it("are Hashtag nodes over the '#' and the name, with their lines", () => {
    expect(nodes("x #tag, y #two/x", "Hashtag")).toEqual(["#tag", "#two/x"]);
    expect(findTags(parsed("one\n\ntwo #a\nthree #b"))).toEqual([
      { name: "a", line: 3 },
      { name: "b", line: 4 },
    ]);
  });
});

describe("highlights", () => {
  it("are Highlight nodes with their marks", () => {
    expect(nodes("a ==marked text== b", "Highlight")).toEqual(["==marked text=="]);
    expect(nodes("a ==marked text== b", "HighlightMark")).toEqual(["==", "=="]);
  });

  it("hold the syntax inside them", () => {
    expect(inside("==a **b** [[c]] #d==", "Highlight")).toEqual([
      "HighlightMark",
      "StrongEmphasis",
      "WikiLink",
      "Hashtag",
      "HighlightMark",
    ]);
  });

  it("run across the lines of a paragraph", () => {
    expect(nodes("one ==two\nthree== four", "Highlight")).toEqual(["==two\nthree=="]);
  });

  it("open before text and close after it, as GFM's ~~ does", () => {
    expect(nodes("== a== and ==b ==", "Highlight")).toEqual([]);
    expect(nodes("x == y == z", "Highlight")).toEqual([]);
    expect(nodes("==unclosed", "Highlight")).toEqual([]);
  });

  it("are not highlights in code", () => {
    expect(nodes("`==code==`\n\n```\n==fenced==\n```\n", "Highlight")).toEqual([]);
  });
});
