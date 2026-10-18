// The spec's parse fixtures (docs/specs/markdown.md#parse-fixtures), read
// for the tests that run them through the editor's parser.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

/** Fixture is a fixture's note and what Parse returns for it. */
export interface Fixture {
  id: string;
  title: string;
  note: string;
  want: {
    links?: { kind?: string; target?: string; subpath?: string; alias?: string; embed?: boolean; line: number }[];
    tags?: { name: string; front?: boolean; line: number }[];
    blocks?: { id: string; line: number }[];
    words?: number;
  };
}

/** readFixtures reads each fixture's note and expected parse from the spec. */
export function readFixtures(): Fixture[] {
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
