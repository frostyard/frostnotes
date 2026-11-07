// The test DOM's `selectionchange` (setup.ts): queued as a browser queues
// it, fired only when the selection moved, and other events left alone.
import { afterEach, describe, expect, it } from "vitest";

const flush = () => new Promise<void>((done) => queueMicrotask(done));

function text(s: string): Text {
  const p = document.createElement("p");
  p.textContent = s;
  document.body.append(p);
  return p.firstChild as Text;
}

function listen(type: string): Event[] {
  const seen: Event[] = [];
  const on = (e: Event) => seen.push(e);
  document.addEventListener(type, on);
  stops.push(() => document.removeEventListener(type, on));
  return seen;
}

const stops: (() => void)[] = [];
afterEach(() => {
  for (const stop of stops.splice(0)) stop();
  document.getSelection()?.removeAllRanges();
  document.body.replaceChildren();
});

describe("selectionchange in the test DOM", () => {
  it("fires after the call that moved the selection, not inside it", async () => {
    const seen = listen("selectionchange");
    const node = text("hello");
    document.getSelection()?.setBaseAndExtent(node, 1, node, 3);
    expect(seen).toHaveLength(0);
    await flush();
    expect(seen).toHaveLength(1);
  });

  it("fires once for a new range at the same points", async () => {
    const seen = listen("selectionchange");
    const node = text("hello");
    const sel = document.getSelection();
    sel?.setBaseAndExtent(node, 2, node, 4);
    await flush();
    sel?.setBaseAndExtent(node, 2, node, 4);
    await flush();
    expect(seen).toHaveLength(1);
  });

  it("fires when the selection moves, turns round or is cleared", async () => {
    const seen = listen("selectionchange");
    const node = text("hello world");
    const sel = document.getSelection();
    sel?.setBaseAndExtent(node, 0, node, 5);
    await flush();
    sel?.setBaseAndExtent(node, 6, node, 11);
    await flush();
    sel?.setBaseAndExtent(node, 11, node, 6);
    await flush();
    sel?.removeAllRanges();
    await flush();
    expect(seen).toHaveLength(4);
  });

  it("dispatches other events at once", () => {
    const seen = listen("frostnotes-test");
    document.dispatchEvent(new Event("frostnotes-test"));
    expect(seen).toHaveLength(1);
  });
});
