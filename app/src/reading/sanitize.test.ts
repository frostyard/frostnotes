// @vitest-environment jsdom
// The app's sanitizer against notesd's contract (docs/specs/render.md):
// every fixture's HTML sanitizes to itself but R02's onclick, as notesd's
// does, and what script, handlers and dangerous URLs try goes. DOMPurify
// misreads happy-dom's DOM (it keeps <script> there), so this runs in
// jsdom, as a browser's DOM.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

import { createSanitizer, escapeHTML, safeURL } from "./sanitize";

const sanitize = createSanitizer();

/** normal serializes HTML through the DOM, as the sanitizer's output is. */
function normal(html: string): string {
  const t = document.createElement("template");
  t.innerHTML = html;
  return t.innerHTML;
}

/** readFixtures reads each fixture's expected HTML from the render spec. */
function readFixtures(): { id: string; html: string }[] {
  const spec = readFileSync(resolve(process.cwd(), "../docs/specs/render.md"), "utf8");
  const lines = spec.split("\n");
  const out: { id: string; html: string }[] = [];
  for (let i = 0; i < lines.length; i++) {
    const head = /^### (R\d\d) — /.exec(lines[i] ?? "");
    if (!head) continue;
    while (i < lines.length && lines[i] !== "```html") i++;
    const body: string[] = [];
    for (i++; i < lines.length && lines[i] !== "```"; i++) body.push(lines[i] ?? "");
    out.push({ id: head[1] ?? "", html: body.join("\n") });
  }
  return out;
}

describe("the reading view's sanitizer", () => {
  const fixtures = readFixtures();

  it("reads the spec's fixtures", () => {
    expect(fixtures.length).toBeGreaterThanOrEqual(18);
  });

  for (const f of fixtures) {
    it(`keeps ${f.id}'s HTML${f.id === "R02" ? ", but its onclick" : ""}`, () => {
      const want = f.id === "R02" ? f.html.replace(' onclick="bad()"', "") : f.html;
      expect(sanitize(f.html)).toBe(normal(want));
    });
  }

  it("takes out what script, handlers, dangerous URLs, forms, frames and foreign content try", () => {
    const cases: [string, string][] = [
      ["a<script>alert(1)</script>b", "ab"],
      ['<img src="https://e.test/a.png" onerror="alert(1)" alt="a">', '<img src="https://e.test/a.png" alt="a">'],
      ['<a href="javascript:alert(1)">x</a><a href=" jav&#x09;ascript:x">y</a>', "<a>x</a><a>y</a>"],
      ['<img src="data:image/svg+xml;base64,AAAA" alt="x">', '<img alt="x">'],
      ['<a href="Notes/Plan.md">a</a>', "<a>a</a>"],
      [
        '<a href="https://e.test/" rel="noopener">w</a><a href="mailto:x@e.test">m</a><a href="#fn:1">1</a>',
        '<a href="https://e.test/" rel="noopener">w</a><a href="mailto:x@e.test">m</a><a href="#fn:1">1</a>',
      ],
      ['<span style="position:fixed">x</span>', "<span>x</span>"],
      ["<form><input type=text><button>go</button></form>z", "z"],
      [
        '<input type="password"><input type="checkbox" checked="" disabled="" data-line="3">',
        '<input type="checkbox" checked="" disabled="" data-line="3">',
      ],
      ['<svg onload="x()"><circle/></svg><iframe src="https://e.test"></iframe>k', "k"],
      ['<custom-el data-x="1">kept</custom-el>', "kept"],
      ['<video controls="" src="https://e.test/v.mp4" autoplay=""></video>', '<video controls=""></video>'],
    ];
    for (const [input, want] of cases) expect(sanitize(input), input).toBe(normal(want));
  });

  it("escapes HTML to text, as it does in an engine whose DOM DOMPurify misreads", () => {
    expect(escapeHTML('<p a="b">&</p>')).toBe("&#60;p a=&#34;b&#34;&#62;&#38;&#60;/p&#62;");
  });

  it("reads a URL's scheme as a browser does", () => {
    expect(safeURL("HTTPS://e.test", true)).toBe(true);
    expect(safeURL("mailto:x@e.test", false)).toBe(false);
    expect(safeURL("#top", false)).toBe(false);
    expect(safeURL("java\u0000script:x", true)).toBe(false);
    expect(safeURL("//evil.test", true)).toBe(false);
  });
});
