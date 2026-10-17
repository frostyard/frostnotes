import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { type TabCommand, type TabItem, TabStrip, type TabStripProps } from "./TabStrip";

const TABS: TabItem[] = [
  { id: "a", title: "Garden plan", active: true, pinned: false },
  { id: "b", title: "Reading list", active: false, pinned: false },
  { id: "c", title: "Inbox", active: false, pinned: true },
];

function setup(over: Partial<TabStripProps> = {}) {
  const commands: TabCommand[] = [];
  const props: TabStripProps = {
    label: "Tabs",
    tabs: TABS,
    focused: true,
    dragRegion: false,
    onCommand: (c) => commands.push(c),
    onDragStart: vi.fn(),
    ...over,
  };
  const view = render(<TabStrip {...props} />);
  return { commands, props, view, user: userEvent.setup() };
}

const tab = (name: string) => screen.getByRole("tab", { name });

describe("TabStrip", () => {
  it("draws a tab list with one tab per tab, the active one selected and focusable", () => {
    setup();
    const list = screen.getByRole("tablist", { name: "Tabs" });
    const tabs = within(list).getAllByRole("tab");
    expect(tabs.map((t) => t.getAttribute("aria-label"))).toEqual(["Garden plan", "Reading list", "Inbox"]);
    expect(tabs.map((t) => t.getAttribute("aria-selected"))).toEqual(["true", "false", "false"]);
    expect(tabs.map((t) => t.tabIndex)).toEqual([0, -1, -1]);
    expect(tabs.map((t) => t.getAttribute("title"))).toEqual(["Garden plan", "Reading list", "Inbox"]);
    expect(tabs.every((t) => t.getAttribute("draggable") === "true")).toBe(true);
    expect(tabs.map((t) => t.getAttribute("data-tab-id"))).toEqual(["a", "b", "c"]);
  });

  it("styles the active tab, weightier in the focused group", () => {
    const { view } = setup();
    expect(tab("Garden plan").className).toContain("bg-window");
    expect(tab("Garden plan").className).toContain("font-medium");
    expect(tab("Reading list").className).toContain("text-secondary");
    expect(tab("Reading list").className).not.toContain("bg-window");
    view.rerender(
      <TabStrip
        label="Tabs"
        tabs={TABS}
        focused={false}
        dragRegion={false}
        onCommand={vi.fn()}
        onDragStart={vi.fn()}
      />,
    );
    expect(tab("Garden plan").className).toContain("bg-window");
    expect(tab("Garden plan").className).not.toContain("font-medium");
  });

  it("gives unpinned tabs a close button out of the tab order, and pinned ones a pin", () => {
    setup();
    const close = screen.getByRole("button", { name: "Close Garden plan" });
    expect(close.tabIndex).toBe(-1);
    expect(screen.getByRole("button", { name: "Close Reading list" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Close Inbox" })).toBeNull();
    expect(within(tab("Inbox")).getByRole("img", { name: "Pinned" })).toBeTruthy();
  });

  it("activates a clicked tab and closes without activating", async () => {
    const { commands, user } = setup();
    await user.click(tab("Reading list"));
    await user.click(screen.getByRole("button", { name: "Close Reading list" }));
    expect(commands).toEqual([
      { kind: "activate", id: "b" },
      { kind: "close", id: "b" },
    ]);
  });

  it("closes a tab on a middle click, unless it is pinned", () => {
    const { commands } = setup();
    fireEvent(tab("Reading list"), new MouseEvent("auxclick", { bubbles: true, button: 1 }));
    fireEvent(tab("Inbox"), new MouseEvent("auxclick", { bubbles: true, button: 1 }));
    fireEvent(tab("Garden plan"), new MouseEvent("auxclick", { bubbles: true, button: 2 }));
    expect(commands).toEqual([{ kind: "close", id: "b" }]);
  });

  it("asks for a tab's menu where it was right-clicked", () => {
    const { commands } = setup();
    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 120, clientY: 30 });
    fireEvent(tab("Inbox"), event);
    expect(event.defaultPrevented).toBe(true);
    expect(commands).toEqual([{ kind: "menu", id: "c", x: 120, y: 30 }]);
  });

  it("offers a new tab and the tab list", async () => {
    const { commands, user } = setup();
    const add = screen.getByRole("button", { name: "New tab" });
    expect(add.getAttribute("title")).toBe("New tab (Ctrl + T)");
    await user.click(add);
    const list = screen.getByRole("button", { name: "Tab list" });
    expect(list.getAttribute("aria-haspopup")).toBe("menu");
    await user.click(list);
    expect(commands).toEqual([{ kind: "newTab" }, { kind: "list", x: expect.any(Number), y: expect.any(Number) }]);
  });

  it("moves between tabs with the arrow keys, Home and End, not past the ends", () => {
    const { commands } = setup();
    const first = tab("Garden plan");
    fireEvent.keyDown(first, { key: "ArrowLeft" });
    fireEvent.keyDown(first, { key: "Home" });
    fireEvent.keyDown(first, { key: "ArrowRight" });
    fireEvent.keyDown(first, { key: "End" });
    expect(commands).toEqual([
      { kind: "activate", id: "b" },
      { kind: "activate", id: "c" },
    ]);
  });

  it("keeps the arrow keys and Home and End from the window's keymap", () => {
    setup();
    for (const key of ["ArrowLeft", "ArrowRight", "Home", "End"]) {
      const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
      fireEvent(tab("Garden plan"), event);
      expect(event.defaultPrevented, key).toBe(true);
    }
  });

  it("moves focus to the tab a key activated once the strip shows it active", () => {
    const onCommand = vi.fn();
    const props = { label: "Tabs", focused: true, dragRegion: false, onCommand, onDragStart: vi.fn() };
    const view = render(<TabStrip {...props} tabs={TABS} />);
    tab("Garden plan").focus();
    fireEvent.keyDown(tab("Garden plan"), { key: "ArrowRight" });
    expect(onCommand).toHaveBeenCalledWith({ kind: "activate", id: "b" });
    const next = TABS.map((t) => ({ ...t, active: t.id === "b" }));
    view.rerender(<TabStrip {...props} tabs={next} />);
    expect(document.activeElement).toBe(tab("Reading list"));
  });

  it("closes the focused tab with Delete, unless it is pinned", () => {
    const { commands, view } = setup();
    fireEvent.keyDown(tab("Garden plan"), { key: "Delete" });
    const pinnedActive = TABS.map((t) => ({ ...t, active: t.id === "c" }));
    view.rerender(
      <TabStrip
        label="Tabs"
        tabs={pinnedActive}
        focused
        dragRegion={false}
        onCommand={(c) => commands.push(c)}
        onDragStart={vi.fn()}
      />,
    );
    fireEvent.keyDown(tab("Inbox"), { key: "Delete" });
    expect(commands).toEqual([{ kind: "close", id: "a" }]);
  });

  it("opens the focused tab's menu with the Menu key and Shift+F10", () => {
    const { commands } = setup();
    fireEvent.keyDown(tab("Garden plan"), { key: "ContextMenu" });
    fireEvent.keyDown(tab("Garden plan"), { key: "F10", shiftKey: true });
    fireEvent.keyDown(tab("Garden plan"), { key: "F10" });
    expect(commands).toEqual([
      { kind: "menu", id: "a", x: expect.any(Number), y: expect.any(Number) },
      { kind: "menu", id: "a", x: expect.any(Number), y: expect.any(Number) },
    ]);
  });

  it("hands a dragged tab to the workspace", () => {
    const onDragStart = vi.fn();
    setup({ onDragStart });
    fireEvent.dragStart(tab("Reading list"));
    expect(onDragStart).toHaveBeenCalledTimes(1);
    expect(onDragStart.mock.calls[0]?.[0]).toBe("b");
  });

  it("drags the window from its empty space only in the title bar row", () => {
    const { view } = setup({ dragRegion: true });
    const list = screen.getByRole("tablist", { name: "Tabs" });
    expect(list.hasAttribute("data-tauri-drag-region")).toBe(true);
    for (const el of [...screen.getAllByRole("tab"), ...screen.getAllByRole("button")]) {
      expect(el.hasAttribute("data-tauri-drag-region"), el.getAttribute("aria-label") ?? "").toBe(false);
    }
    view.unmount();
    setup({ dragRegion: false });
    expect(document.querySelector("[data-tauri-drag-region]")).toBeNull();
  });
});
