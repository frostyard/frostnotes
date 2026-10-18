// The spec's parse fixtures (docs/specs/markdown.md#parse-fixtures) through
// the editor's parser: where the two parsers must agree, on what is a link
// or a tag and where code hides them, Lezer finds what goldmark finds. A
// fixture that needs an extension batch 3 has not built yet is PENDING,
// and must fail until that card lands; the card then removes it here.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import { findLinks, findTags, frontmatterEnd } from "./extract";
import { markdownSupport } from "./index";

/** PENDING names the fixtures that wait on a batch 3 extension, and which. */
const PENDING: Record<string, string> = {
  F01: "embeds",
  F02: "tags and math",
  F05: "tags",
  F06: "tags",
  F08: "tags",
  F09: "tags",
  F12: "tags, highlights, comments and callouts",
};

interface Fixture {
  id: string;
  title: string;
  note: string;
  want: {
    links?: { kind?: string; target?: string; subpath?: string; alias?: string; embed?: boolean; line: number }[];
    tags?: { name: string; front?: boolean; line: number }[];
  };
}

/** readFixtures reads each fixture's note and expected parse from the spec. */
function readFixtures(): Fixture[] {
  // Vitest runs in app/; the spec is the repository's.
  const spec = readFileSync(resolve(process.cwd(), "../docs/specs/markdown.md"), "utf8");
  const lines = spec.split("\n");
  const out: Fixture[] = [];
  for (let i = 0; i < lines.length; i++) {
    const head = /^### (F\d\d) — (.*)$/.exec(lines[i] ?? "");
    if (!head) continue;
    const block = (info: string): string => {
      while (i < lines.length && !new RegExp(`^(\`{3,})${info}$`).test(lines[i] ?? "")) i++;
      const fence = /^`+/.exec(lines[i] ?? "")?.[0] ?? "```";
      const body: string[] = [];
      for (i++; i < lines.length && lines[i] !== fence; i++) body.push(lines[i] ?? "");
      return `${body.join("\n")}\n`;
    };
    const note = block("md");
    const want = JSON.parse(block("json")) as Fixture["want"];
    out.push({ id: head[1] ?? "", title: head[2] ?? "", note, want });
  }
  return out;
}

function parse(note: string) {
  const state = EditorState.create({ doc: note, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  const body = frontmatterEnd(note);
  return {
    links: findLinks(state)
      .filter((l) => l.from >= body)
      .map(({ kind, target, subpath, alias, embed, line }) => ({ kind, target, subpath, alias, embed, line })),
    tags: findTags(state).filter((t) => state.doc.line(t.line).from >= body),
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
    const run = f.id in PENDING ? it.fails : it;
    run(`${f.id} — ${f.title}${f.id in PENDING ? ` (waits on ${PENDING[f.id]})` : ""}`, () => {
      const got = parse(f.note);
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
    });
  }
});
