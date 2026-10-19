import { afterEach, describe, expect, it, vi } from "vitest";

import { installEdgeSwipe, installLongPress } from "./touch";

// pointer dispatches a pointer event of type at (x, y) on el.
function pointer(el: Element, type: string, x: number, y: number, pointerType = "touch") {
  const e = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y });
  Object.defineProperty(e, "pointerType", { value: pointerType });
  el.dispatchEvent(e);
}

afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = "";
});

describe("a long press", () => {
  it("opens the context menu at its point, and swallows the click that ends it", () => {
    vi.useFakeTimers();
    const row = document.createElement("div");
    document.body.append(row);
    const menus: [number, number][] = [];
    row.addEventListener("contextmenu", (e) => menus.push([e.clientX, e.clientY]));
    let clicks = 0;
    row.addEventListener("click", () => clicks++);
    const off = installLongPress(window, 500);
    pointer(row, "pointerdown", 40, 60);
    vi.advanceTimersByTime(499);
    expect(menus).toEqual([]);
    vi.advanceTimersByTime(1);
    expect(menus).toEqual([[40, 60]]);
    pointer(row, "pointerup", 40, 60);
    row.click();
    expect(clicks).toBe(0);
    row.click();
    expect(clicks).toBe(1);
    off();
  });

  it("is no long press when the touch moves, ends early, is a mouse, or is in the editor", () => {
    vi.useFakeTimers();
    const row = document.createElement("div");
    const editor = document.createElement("div");
    editor.className = "cm-editor";
    const line = document.createElement("div");
    editor.append(line);
    document.body.append(row, editor);
    let menus = 0;
    document.addEventListener("contextmenu", () => menus++);
    const off = installLongPress(window, 500);
    pointer(row, "pointerdown", 40, 60);
    pointer(row, "pointermove", 40, 75);
    vi.advanceTimersByTime(600);
    pointer(row, "pointerdown", 40, 60);
    pointer(row, "pointerup", 40, 60);
    vi.advanceTimersByTime(600);
    pointer(row, "pointerdown", 40, 60, "mouse");
    vi.advanceTimersByTime(600);
    pointer(line, "pointerdown", 40, 60);
    vi.advanceTimersByTime(600);
    expect(menus).toBe(0);
    off();
  });
});

describe("an edge swipe", () => {
  it("opens the drawer of the edge it starts at, moving inward", () => {
    const opened: string[] = [];
    const off = installEdgeSwipe((side) => opened.push(side));
    const w = window.innerWidth;
    pointer(document.body, "pointerdown", 5, 300);
    pointer(document.body, "pointermove", 60, 310);
    pointer(document.body, "pointerup", 60, 310);
    pointer(document.body, "pointerdown", w - 5, 300);
    pointer(document.body, "pointermove", w - 70, 300);
    // From the middle, outward, or more down than across: nothing.
    pointer(document.body, "pointerdown", 200, 300);
    pointer(document.body, "pointermove", 300, 300);
    pointer(document.body, "pointerdown", 5, 300);
    pointer(document.body, "pointermove", 40, 400);
    expect(opened).toEqual(["left", "right"]);
    off();
  });
});
