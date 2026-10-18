// T-0065's given tests: embeds, block IDs and footnotes in the editor's parser.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import { findBlocks, findLinks } from "./extract";
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

/** children lists the names of the nodes directly inside each node named name. */
function children(doc: string, name: string): string[][] {
  const state = parsed(doc);
  const out: string[][] = [];
  syntaxTree(state).iterate({
    enter: (n) => {
      if (n.name !== name) return;
      const kids: string[] = [];
      for (let c = n.node.firstChild; c; c = c.nextSibling) kids.push(c.name);
      out.push(kids);
    },
  });
  return out;
}

const links = (doc: string) =>
  findLinks(parsed(doc)).map(({ target, subpath, alias, embed, from, to }) => ({
    target,
    subpath,
    alias,
    embed,
    from,
    to,
  }));
const blocks = (doc: string) => findBlocks(parsed(doc)).map(({ id, line }) => ({ id, line }));

describe("embeds", () => {
  it("are Embed nodes holding the '!' and the wikilink", () => {
    expect(nodes("See ![[soil-map.png]] here", "Embed")).toEqual(["![[soil-map.png]]"]);
    expect(nodes("See ![[soil-map.png]] here", "EmbedMark")).toEqual(["!"]);
    expect(children("![[Plan#Goals|the goals]]", "Embed")).toEqual([["EmbedMark", "WikiLink"]]);
  });

  it("are links that embed, from the '!'", () => {
    expect(links("A ![[Plan#Goals|the goals]] and [[Plan]]")).toEqual([
      { target: "Plan", subpath: "#Goals", alias: "the goals", embed: true, from: 2, to: 27 },
      { target: "Plan", subpath: "", alias: "", embed: false, from: 32, to: 40 },
    ]);
  });

  it("work in a table cell, its alias after '\\|'", () => {
    const doc = "| Map |\n| --- |\n| ![[map.png\\|300]] |\n";
    expect(links(doc).map(({ target, alias, embed }) => ({ target, alias, embed }))).toEqual([
      { target: "map.png", alias: "300", embed: true },
    ]);
  });

  it("need a link right after the '!'", () => {
    expect(nodes("! [[a]] and !x and ![[]] and !![[b]]", "Embed")).toEqual(["![[b]]"]);
    expect(links("\\![[escaped]]").map((l) => l.embed)).toEqual([false]);
  });

  it("leave markdown images and code alone", () => {
    expect(nodes("![alt](map.png) `![[code]]`", "Embed")).toEqual([]);
    expect(nodes("![alt](map.png)", "Image")).toEqual(["![alt](map.png)"]);
  });
});

describe("block IDs", () => {
  it("end a paragraph or a list item's text after white space", () => {
    const doc = "A paragraph. ^para-1\n\n- item ^item-a  \n- other\n\n> quoted ^Q9\n\nOne line\n^next-line\n";
    expect(blocks(doc)).toEqual([
      { id: "para-1", line: 1 },
      { id: "item-a", line: 3 },
      { id: "Q9", line: 6 },
      { id: "next-line", line: 9 },
    ]);
    expect(nodes("text ^abc-1", "BlockId")).toEqual(["^abc-1"]);
  });

  it("stand alone only right after a list, a quote or a table", () => {
    const doc =
      "- a\n\n^after-list\n\n> q\n\n\n^after-quote\n\n| a |\n| - |\n| 1 |\n\n^after-table\n\npara\n\n^after-para\n";
    expect(blocks(doc)).toEqual([
      { id: "after-list", line: 3 },
      { id: "after-quote", line: 8 },
      { id: "after-table", line: 14 },
    ]);
  });

  it("are not IDs elsewhere", () => {
    const doc =
      "`^code` text\n\nx ^not-at-end y\n\nx^no-space\n\n## Heading ^head\n\nx ^bad_id\n\n```\nfenced ^fenced\n```\n";
    expect(blocks(doc)).toEqual([]);
    expect(nodes("x ^not-at-end y\n\nx^no-space\n\nx ^bad_id\n\nx ^ \n", "BlockId")).toEqual([]);
  });

  it("are not taken from links", () => {
    expect(blocks("See [[Soil#^ph-test|pH]]")).toEqual([]);
  });
});

describe("footnotes", () => {
  it("parse references with their marks and labels", () => {
    const doc = "Text[^1] and [^long-note].\n";
    expect(nodes(doc, "FootnoteReference")).toEqual(["[^1]", "[^long-note]"]);
    expect(nodes(doc, "FootnoteLabel")).toEqual(["1", "long-note"]);
    expect(children(doc, "FootnoteReference")).toEqual([
      ["FootnoteMark", "FootnoteLabel", "FootnoteMark"],
      ["FootnoteMark", "FootnoteLabel", "FootnoteMark"],
    ]);
  });

  it("need a label without spaces or brackets", () => {
    expect(nodes("[^] [^ 1] [^a b] `[^1]`", "FootnoteReference")).toEqual([]);
  });

  it("parse a definition's line, its text as usual, and not as a link reference", () => {
    const doc = "Body.[^1]\n\n[^1]: The *first* note, see [[Plan]].\nNext line.\n";
    expect(nodes(doc, "FootnoteDefinition")).toEqual(["[^1]: The *first* note, see [[Plan]]."]);
    expect(nodes(doc, "LinkReference")).toEqual([]);
    expect(children(doc, "FootnoteDefinition")).toEqual([
      ["FootnoteMark", "FootnoteLabel", "FootnoteMark", "Emphasis", "WikiLink"],
    ]);
    expect(nodes(doc, "FootnoteMark")).toEqual(["[^", "]", "[^", "]:"]);
    expect(findLinks(parsed(doc)).map((l) => [l.target, l.line])).toEqual([["Plan", 3]]);
    expect(nodes(doc, "Paragraph")).toEqual(["Body.[^1]", "Next line."]);
  });

  it("keep link references working", () => {
    const doc = "A [ref][r].\n\n[r]: Recipes/Apple%20Butter.md\n";
    expect(nodes(doc, "LinkReference")).toEqual(["[r]: Recipes/Apple%20Butter.md"]);
    expect(nodes(doc, "FootnoteDefinition")).toEqual([]);
  });
});
