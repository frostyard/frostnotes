// @vitest-environment jsdom
// What finish and fill do with sanitized HTML (review findings 6 and 8,
// plan 0005 Phase 7): a remote image the preference does not load never
// reaches the page with its URL, and math KaTeX cannot draw shows as its
// TeX without stopping the rest. KaTeX needs a standards-mode document,
// so this runs in jsdom.
import katex from "katex";
import { afterEach, describe, expect, it, vi } from "vitest";

import { fill, finish } from "./finish";

const blobURL = (hash: string, ext: string) => `notesblob://localhost/${hash}.${ext}`;

afterEach(() => {
  vi.restoreAllMocks();
});

describe("fill", () => {
  it("finishes the HTML before the page has it", () => {
    const el = document.createElement("div");
    document.body.append(el);
    const set = vi.spyOn(el, "innerHTML", "set");
    fill(
      el,
      '<p><img src="https://tracker.test/p.png?u=42" alt="t"><img class="embed" data-blob="ab.png" alt="b"></p>',
      blobURL,
      false,
    );
    expect(set).not.toHaveBeenCalled();
    const [remote, local] = el.querySelectorAll("img");
    expect(remote?.hasAttribute("src")).toBe(false);
    expect(local?.getAttribute("src")).toBe("notesblob://localhost/ab.png");
    el.remove();
  });

  it("keeps remote images when the preference loads them", () => {
    const el = document.createElement("div");
    fill(el, '<img src="https://e.test/a.png" alt="a">', blobURL, true);
    expect(el.querySelector("img")?.getAttribute("src")).toBe("https://e.test/a.png");
  });
});

describe("finish", () => {
  it("shows math KaTeX cannot draw as its TeX, and draws the rest", () => {
    const render = katex.render.bind(katex);
    vi.spyOn(katex, "render").mockImplementation((tex, node, opts) => {
      if (tex === "bad") throw new RangeError("Maximum call stack size exceeded");
      render(tex, node, opts);
    });
    const el = document.createElement("div");
    el.innerHTML =
      '<span class="math" data-tex="bad"></span><span class="math" data-tex="x^2"></span><img data-blob="cd.png">';
    expect(() => finish(el, blobURL, true)).not.toThrow();
    const [bad, good] = el.querySelectorAll<HTMLElement>(".math");
    expect(bad?.textContent).toBe("bad");
    expect(good?.querySelector(".katex")).not.toBeNull();
    expect(el.querySelector("img")?.getAttribute("src")).toBe("notesblob://localhost/cd.png");
  });
});
