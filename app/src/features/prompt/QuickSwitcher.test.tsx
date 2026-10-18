import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { SwitcherNote } from "../../lib/switcher";
import { QuickSwitcher, type QuickSwitcherProps, whereFor } from "./QuickSwitcher";

const NOTES: SwitcherNote[] = [
  { path: "Inbox.md", aliases: [] },
  { path: "Projects/Garden plan.md", aliases: ["Allotment"] },
  { path: "Plans/Weekly.md", aliases: [] },
];

function setup(over: Partial<QuickSwitcherProps> = {}) {
  const props: QuickSwitcherProps = {
    notes: NOTES,
    recent: ["Plans/Weekly.md", "Inbox.md"],
    onOpen: vi.fn(),
    onCreate: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(<QuickSwitcher {...props} />);
  return { props, user: userEvent.setup() };
}

const field = () => screen.getByRole("combobox", { name: "Quick switcher" });
const options = () => within(screen.getByRole("listbox")).queryAllByRole("option");
const accents = (el: Element) => [...el.querySelectorAll(".text-accent")].map((s) => s.textContent);

describe("whereFor", () => {
  it("opens to the right with Ctrl+Alt, in a new tab with Ctrl, else here", () => {
    expect(whereFor({ ctrl: false, alt: false, shift: false })).toBe("current");
    expect(whereFor({ ctrl: true, alt: false, shift: false })).toBe("tab");
    expect(whereFor({ ctrl: true, alt: true, shift: false })).toBe("right");
    expect(whereFor({ ctrl: false, alt: true, shift: false })).toBe("current");
  });
});

describe("QuickSwitcher", () => {
  it("opens as the quick switcher, listing recent notes by name with their folders", () => {
    setup();
    expect(screen.getByRole("dialog", { name: "Quick switcher" })).toBeTruthy();
    expect(field().getAttribute("placeholder")).toBe("Find or create a note…");
    expect(options().map((o) => o.textContent)).toEqual(["WeeklyPlans", "Inbox"]);
  });

  it("says there are no recent files", () => {
    setup({ recent: [] });
    expect(screen.getByText("No recent files.")).toBeTruthy();
  });

  it("matches as typed, highlighting the matched characters in the name and the folder", async () => {
    const { user } = setup();
    await user.type(field(), "plan");
    const rows = options();
    expect(rows.map((o) => o.textContent)).toEqual(["Garden planProjects", "WeeklyPlans"]);
    expect(accents(rows[0] as HTMLElement)).toEqual(["plan"]);
    expect(accents(rows[1] as HTMLElement)).toEqual(["Plan"]);
  });

  it("shows an alias with the note it names", async () => {
    const { user } = setup();
    await user.type(field(), "allot");
    const [row] = options();
    expect(row?.textContent).toBe("AllotmentGarden plan");
    expect(accents(row as HTMLElement)).toEqual(["Allot"]);
    expect(row?.querySelector("svg")).not.toBeNull();
  });

  it("opens the selected note here, in a new tab, or to the right, closing first", async () => {
    const order: string[] = [];
    const onOpen = vi.fn(() => order.push("open"));
    const onClose = vi.fn(() => order.push("close"));
    const { user } = setup({ onOpen, onClose });
    await user.type(field(), "plan");
    fireEvent.keyDown(field(), { key: "Enter" });
    fireEvent.keyDown(field(), { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(field(), { key: "Enter", ctrlKey: true, altKey: true });
    expect(onOpen.mock.calls).toEqual([
      ["Projects/Garden plan.md", "current"],
      ["Projects/Garden plan.md", "tab"],
      ["Projects/Garden plan.md", "right"],
    ]);
    expect(order.slice(0, 2)).toEqual(["close", "open"]);
  });

  it("opens an alias's note, and a clicked row", async () => {
    const onOpen = vi.fn();
    const { user } = setup({ onOpen });
    await user.type(field(), "allot");
    fireEvent.keyDown(field(), { key: "Enter" });
    fireEvent.click(options()[0] as HTMLElement, { ctrlKey: true });
    expect(onOpen.mock.calls).toEqual([
      ["Projects/Garden plan.md", "current"],
      ["Projects/Garden plan.md", "tab"],
    ]);
  });

  it("creates a note named as typed with Shift+Enter, or Enter when nothing matches", async () => {
    const onCreate = vi.fn();
    const onOpen = vi.fn();
    const { user } = setup({ onCreate, onOpen });
    await user.type(field(), "plan");
    fireEvent.keyDown(field(), { key: "Enter", shiftKey: true });
    await user.clear(field());
    await user.type(field(), "  Seed swap  ");
    expect(screen.getByText("No notes found.")).toBeTruthy();
    fireEvent.keyDown(field(), { key: "Enter", ctrlKey: true });
    expect(onCreate.mock.calls).toEqual([
      ["plan", "current"],
      ["Seed swap", "tab"],
    ]);
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("creates nothing from an empty query", () => {
    const onCreate = vi.fn();
    const onClose = vi.fn();
    setup({ onCreate, onClose, recent: [] });
    fireEvent.keyDown(field(), { key: "Enter" });
    fireEvent.keyDown(field(), { key: "Enter", shiftKey: true });
    expect(onCreate).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("lists its keys in the footer", () => {
    setup();
    for (const t of [
      "to navigate",
      "to open",
      "to open in new tab",
      "to open to the right",
      "to create",
      "to dismiss",
    ]) {
      expect(screen.getByText(t)).toBeTruthy();
    }
  });
});
