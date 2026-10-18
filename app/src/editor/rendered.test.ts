// The block renderer behind live preview's rendered blocks: what it
// caches, what it fills again, and what it drops.
import { describe, expect, it } from "vitest";

import { BlockRenderer, RENDER_CACHE } from "./rendered";

/** setup is a renderer whose answers the test gives by hand. */
function setup() {
  const asked: string[] = [];
  const waiting = new Map<string, { resolve: (html: string) => void; reject: (err: Error) => void }>();
  let filled = 0;
  const finished: HTMLElement[] = [];
  const r = new BlockRenderer(
    (markdown) =>
      new Promise((resolve, reject) => {
        asked.push(markdown);
        waiting.set(markdown, { resolve, reject });
      }),
    (el) => void finished.push(el),
    () => void filled++,
  );
  const answer = async (markdown: string, html: string) => {
    waiting.get(markdown)?.resolve(html);
    await Promise.resolve();
    await Promise.resolve();
  };
  const refuse = async (markdown: string, err: string) => {
    waiting.get(markdown)?.reject(new Error(err));
    await Promise.resolve();
    await Promise.resolve();
  };
  return { r, asked, answer, refuse, finished, filled: () => filled };
}

const box = () => {
  const el = document.createElement("div");
  document.body.append(el);
  return el;
};

describe("the block renderer", () => {
  it("fills an element when notesd answers, finishing it", async () => {
    const { r, asked, answer, finished, filled } = setup();
    const el = box();
    r.fill(el, "> [!note] A");
    expect(el.innerHTML).toBe("");
    await answer("> [!note] A", "<p>A</p>");
    expect(el.innerHTML).toBe("<p>A</p>");
    expect(finished).toEqual([el]);
    expect(filled()).toBe(1);
    expect(asked).toEqual(["> [!note] A"]);
  });

  it("asks once for the same markdown, and fills from its cache after", async () => {
    const { r, asked, answer } = setup();
    const a = box();
    const b = box();
    r.fill(a, "| x |");
    r.fill(b, "| x |");
    await answer("| x |", "<table></table>");
    const c = box();
    r.fill(c, "| x |");
    expect([a.innerHTML, b.innerHTML, c.innerHTML]).toEqual(["<table></table>", "<table></table>", "<table></table>"]);
    expect(asked).toEqual(["| x |"]);
  });

  it("drops an answer for markdown an element no longer shows", async () => {
    const { r, answer } = setup();
    const el = box();
    r.fill(el, "old");
    r.fill(el, "new");
    await answer("old", "<p>old</p>");
    expect(el.innerHTML).toBe("");
    await answer("new", "<p>new</p>");
    expect(el.innerHTML).toBe("<p>new</p>");
  });

  it("says why it cannot render", async () => {
    const { r, refuse } = setup();
    const el = box();
    r.fill(el, "![[Gone]]");
    await refuse("![[Gone]]", "Gone.md is gone");
    expect(el.textContent).toBe("Cannot render this: Gone.md is gone");
  });

  it("fills what is on screen again on a refresh, keeping it until the answer", async () => {
    const { r, asked, answer } = setup();
    const shown = box();
    const gone = document.createElement("div");
    r.fill(shown, "![[Plan]]");
    r.fill(gone, "![[Other]]");
    await answer("![[Plan]]", "<p>quince</p>");
    r.refresh();
    expect(shown.innerHTML).toBe("<p>quince</p>");
    expect(asked).toEqual(["![[Plan]]", "![[Other]]", "![[Plan]]"]);
    await answer("![[Plan]]", "<p>pears</p>");
    expect(shown.innerHTML).toBe("<p>pears</p>");
  });

  it("keeps at most RENDER_CACHE answers", async () => {
    const { r, asked, answer } = setup();
    for (let i = 0; i <= RENDER_CACHE; i++) {
      r.fill(box(), `md ${i}`);
      await answer(`md ${i}`, `<p>${i}</p>`);
    }
    r.fill(box(), "md 0");
    expect(asked.filter((m) => m === "md 0")).toHaveLength(2);
    r.fill(box(), `md ${RENDER_CACHE}`);
    expect(asked.filter((m) => m === `md ${RENDER_CACHE}`)).toHaveLength(1);
  });
});
