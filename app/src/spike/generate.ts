// A generated markdown note for spike 1 (docs/plans/0001-m0-foundations.md,
// Phase 5): frontmatter, headings, paragraphs with emphasis, links and
// wikilinks, lists, tasks, quotes and fenced code, made-up and deterministic.

const WORDS = (
  "frost lantern meadow quill harbor ember thistle copper orbit willow cinder " +
  "marble glacier fable pebble sorrel tundra velvet beacon cobalt drift hollow " +
  "juniper keel larch mosaic nimbus opal prairie quartz ripple saffron timber " +
  "umber vesper wren yarrow zephyr atlas bramble cairn delta fjord gable heron"
).split(" ");

const NOTES = ["Garden Plan", "Reading List", "Weekly Review", "Project Atlas", "Recipes", "Travel 2026", "Ideas"];

/** rng returns a deterministic generator of numbers in [0, 1) (mulberry32). */
export function rng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

class Writer {
  constructor(readonly r: () => number) {}

  int(lo: number, hi: number): number {
    return lo + Math.floor(this.r() * (hi - lo + 1));
  }

  pick<T>(list: readonly T[]): T {
    return list[Math.floor(this.r() * list.length)] as T;
  }

  words(n: number): string {
    return Array.from({ length: n }, () => this.pick(WORDS)).join(" ");
  }

  // inline is one phrase of prose, sometimes marked up.
  inline(): string {
    const w = this.words(this.int(1, 3));
    switch (this.int(0, 11)) {
      case 0:
        return `*${w}*`;
      case 1:
        return `**${w}**`;
      case 2:
        return `~~${w}~~`;
      case 3:
        return `\`${w.replaceAll(" ", "_")}\``;
      case 4:
        return `[${w}](https://example.test/${this.pick(WORDS)})`;
      case 5:
        return `[[${this.pick(NOTES)}]]`;
      case 6:
        return `[[${this.pick(NOTES)}|${w}]]`;
      case 7:
        return `[[${this.pick(NOTES)}#${this.pick(WORDS)}]]`;
      case 8:
        return `_${w}_`;
      default:
        return w;
    }
  }

  sentence(): string {
    const parts = Array.from({ length: this.int(4, 9) }, () => this.inline());
    const s = parts.join(" ");
    return `${s.charAt(0).toUpperCase()}${s.slice(1)}.`;
  }

  paragraph(): string[] {
    return Array.from({ length: this.int(1, 3) }, () =>
      Array.from({ length: this.int(1, 3) }, () => this.sentence()).join(" "),
    );
  }

  block(): string[] {
    switch (this.int(0, 9)) {
      case 0:
        return [`${"#".repeat(this.int(2, 4))} ${this.words(this.int(2, 4))}`];
      case 1:
      case 2: {
        const out: string[] = [];
        for (let i = this.int(2, 6); i > 0; i--) {
          out.push(`- ${this.sentence()}`);
          if (this.r() < 0.3) out.push(`  - ${this.sentence()}`);
        }
        return out;
      }
      case 3:
        return Array.from({ length: this.int(2, 5) }, (_, i) => `${i + 1}. ${this.sentence()}`);
      case 4:
      case 5:
        return Array.from({ length: this.int(2, 6) }, () => `- [${this.r() < 0.4 ? "x" : " "}] ${this.sentence()}`);
      case 6: {
        const lang = this.pick(["ts", "go", "sh", ""]);
        const body = Array.from({ length: this.int(3, 10) }, (_, i) => {
          const name = this.pick(WORDS);
          return `${"  ".repeat(i % 3)}const ${name}${i} = "${this.words(2)}"; // [[not a link]] *not emphasis*`;
        });
        return [`\`\`\`${lang}`, ...body, "```"];
      }
      case 7:
        return Array.from({ length: this.int(1, 3) }, () => `> ${this.sentence()}`);
      default:
        return this.paragraph();
    }
  }
}

/**
 * generateNote returns a note of exactly lines lines (as CodeMirror counts them),
 * the same for the same seed.
 */
export function generateNote(lines = 10_000, seed = 1): string {
  const w = new Writer(rng(seed));
  const out = ["---", "title: Generated Note", "tags: [spike, cm6]", "aliases: [Big Note]", "---", ""];
  let section = 0;
  while (out.length < lines) {
    const block = out.length === 6 || w.r() < 0.06 ? [`# Section ${++section}: ${w.words(w.int(1, 3))}`] : w.block();
    if (out.length + block.length + 1 > lines) break;
    out.push(...block, "");
  }
  while (out.length < lines) out.push(out.length % 2 ? "" : w.sentence());
  return out.slice(0, lines).join("\n");
}
