// The editor's extensions together (batch 3's T-0064 to T-0067): each
// card's tests take its extension alone; these take them as the editor
// runs them.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { GFM, parser } from "@lezer/markdown";
import { describe, expect, it } from "vitest";

import { countWords } from "../words";
import { findComments } from "./comments";
import { findLinks, findTags } from "./extract";
import { markdownSupport, OBSIDIAN } from "./index";

function parsed(doc: string): EditorState {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  return state;
}

function names(doc: string): string[] {
  const out: string[] = [];
  syntaxTree(parsed(doc)).iterate({ enter: (n) => void out.push(n.name) });
  return out;
}

describe("the editor's parser", () => {
  it("defines every node the extensions name", () => {
    const defined = new Set(parser.configure([GFM, ...OBSIDIAN]).nodeSet.types.map((t) => t.name));
    for (const n of [
      "Frontmatter",
      "WikiLink",
      "Embed",
      "Hashtag",
      "Highlight",
      "InlineMath",
      "BlockMath",
      "BlockId",
      "FootnoteReference",
      "FootnoteDefinition",
    ]) {
      expect(defined.has(n), n).toBe(true);
    }
  });

  it("opens no comment in math", () => {
    const doc = "Inline $a %% b$ and\n\n$$\nx %% y\n$$\n\nthen %%real%% end";
    expect(findComments(parsed(doc)).map((c) => doc.slice(c.from, c.to))).toEqual(["%%real%%"]);
    expect(countWords(parsed(doc)).words).toBe(8);
  });

  it("hides tags in math but not in highlights, callouts or footnotes", () => {
    const doc =
      "$#no$ ==a #yes== x\n\n> [!note] Title\n> body #callout\n\nRef[^1].\n\n[^1]: A #footnote and [[Plan]].\n";
    expect(findTags(parsed(doc)).map((t) => t.name)).toEqual(["yes", "callout", "footnote"]);
    expect(findLinks(parsed(doc)).map((l) => l.target)).toEqual(["Plan"]);
  });

  it("parses an embed, a tag and a block ID on one line", () => {
    expect(names("See ![[map.png]] #map ^fig-1").filter((n) => /Embed|Hashtag|BlockId/.test(n))).toEqual([
      "Embed",
      "EmbedMark",
      "Hashtag",
      "BlockId",
    ]);
  });
});
