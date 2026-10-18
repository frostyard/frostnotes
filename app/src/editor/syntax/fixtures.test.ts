// The spec's parse fixtures (docs/specs/markdown.md#parse-fixtures) through
// the editor's parser: where the two parsers must agree, on what is a link,
// a tag or a block ID and where code and math hide them, Lezer finds what
// goldmark finds. A fixture is pending while an extension it needs has no
// nodes yet (a stub), and must fail until that extension lands.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { GFM, parser } from "@lezer/markdown";
import { describe, expect, it } from "vitest";

import { readFixtures } from "../../testing/fixtures";
import { findBlocks, findLinks, findTags, frontmatterEnd } from "./extract";
import { markdownSupport, OBSIDIAN } from "./index";

/** NEEDS names the nodes each fixture needs beyond the wikilinks. */
const NEEDS: Record<string, string[]> = {
  F01: ["Embed"],
  F02: ["Hashtag", "InlineMath", "BlockMath"],
  F05: ["Hashtag"],
  F06: ["Hashtag"],
  F08: ["Hashtag"],
  F09: ["Hashtag"],
  F10: ["BlockId"],
  F12: ["Hashtag"],
};

const defined = new Set(parser.configure([GFM, ...OBSIDIAN]).nodeSet.types.map((t) => t.name));
const missing = (id: string) => (NEEDS[id] ?? []).filter((n) => !defined.has(n));

function parse(note: string, front: readonly string[]) {
  const state = EditorState.create({ doc: note, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  const body = frontmatterEnd(note);
  // A note lists each tag once, ignoring case, frontmatter tags first.
  const seen = new Set(front.map((t) => t.toLowerCase()));
  return {
    links: findLinks(state)
      .filter((l) => l.from >= body)
      .map(({ kind, target, subpath, alias, embed, line }) => ({ kind, target, subpath, alias, embed, line })),
    tags: findTags(state).filter((t) => {
      if (state.doc.line(t.line).from < body || seen.has(t.name.toLowerCase())) return false;
      seen.add(t.name.toLowerCase());
      return true;
    }),
    blocks: findBlocks(state).map(({ id, line }) => ({ id, line })),
  };
}

const fixtures = readFixtures();

describe("the spec's parse fixtures through Lezer", () => {
  it("reads every fixture", () => {
    expect(fixtures.map((f) => f.id)).toEqual(
      Array.from({ length: 14 }, (_, i) => `F${String(i + 1).padStart(2, "0")}`),
    );
  });

  for (const f of fixtures) {
    const waits = missing(f.id);
    const run = waits.length > 0 ? it.fails : it;
    run(`${f.id} — ${f.title}${waits.length > 0 ? ` (waits on ${waits.join(", ")})` : ""}`, () => {
      const front = (f.want.tags ?? []).filter((t) => t.front).map((t) => t.name.replace(/^#/, ""));
      const got = parse(f.note, front);
      const links = (f.want.links ?? [])
        .filter((l) => l.kind !== "property")
        .map((l) => ({
          kind: l.kind ?? "wiki",
          target: l.target ?? "",
          subpath: l.subpath ?? "",
          alias: l.alias ?? "",
          embed: l.embed ?? false,
          line: l.line,
        }));
      const tags = (f.want.tags ?? []).filter((t) => !t.front).map(({ name, line }) => ({ name, line }));
      expect(got.links).toEqual(links);
      expect(got.tags).toEqual(tags);
      expect(got.blocks).toEqual(f.want.blocks ?? []);
    });
  }
});
