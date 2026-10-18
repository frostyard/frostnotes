import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { buildOutline, type Heading, outlineRows } from "../../lib/outline";
import { OutlinePane, type OutlinePaneProps } from "./OutlinePane";

const HEADINGS: Heading[] = [
  { level: 1, text: "Orchard plan", from: 0 },
  { level: 2, text: "Goals", from: 20 },
  { level: 3, text: "This year", from: 40 },
  { level: 2, text: "Later", from: 60 },
];
const rows = (folded: number[] = []) => outlineRows(buildOutline(HEADINGS), new Set(folded));

function setup(over: Partial<OutlinePaneProps> = {}) {
  const props: OutlinePaneProps = { rows: rows(), current: -1, onToggle: vi.fn(), onGo: vi.fn(), ...over };
  const view = render(<OutlinePane {...props} />);
  return { props, view };
}

const items = () => within(screen.getByRole("tree", { name: "Outline" })).getAllByRole("treeitem");

describe("OutlinePane", () => {
  it("lists the headings as a tree, nested by level", () => {
    setup();
    expect(items().map((i) => i.textContent)).toEqual(["Orchard plan", "Goals", "This year", "Later"]);
    expect(items().map((i) => i.getAttribute("aria-level"))).toEqual(["1", "2", "3", "2"]);
    expect(items().map((i) => i.style.paddingLeft)).toEqual(["8px", "24px", "40px", "24px"]);
    expect(items().map((i) => i.getAttribute("aria-expanded"))).toEqual(["true", "true", null, null]);
  });

  it("marks the heading the cursor is under", () => {
    setup({ current: 2 });
    expect(items().map((i) => i.getAttribute("aria-selected"))).toEqual(["false", "false", "true", "false"]);
    expect(items()[2]?.className).toContain("bg-selection-sidebar");
    expect(items()[1]?.className).not.toContain("bg-selection-sidebar");
  });

  it("gives a chevron only to headings with headings under them, and folds by its start", () => {
    const { props } = setup();
    expect(screen.getAllByRole("button", { name: /^Fold / }).map((b) => b.getAttribute("aria-label"))).toEqual([
      "Fold Orchard plan",
      "Fold Goals",
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Fold Goals" }));
    expect(props.onToggle).toHaveBeenCalledWith(20);
    expect(props.onGo).not.toHaveBeenCalled();
  });

  it("draws folded headings as folded", () => {
    setup({ rows: rows([20]) });
    expect(items().map((i) => i.textContent)).toEqual(["Orchard plan", "Goals", "Later"]);
    expect(items()[1]?.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("button", { name: "Unfold Goals" })).toBeTruthy();
  });

  it("goes to a heading when its name is clicked", () => {
    const { props } = setup();
    fireEvent.click(screen.getByRole("button", { name: "This year" }));
    expect(props.onGo).toHaveBeenCalledWith(HEADINGS[2]);
  });

  it("says when there are no headings, and when no note is open", () => {
    const { view } = setup({ rows: [] });
    expect(screen.getByText("No headings")).toBeTruthy();
    expect(screen.queryByRole("tree")).toBeNull();
    view.unmount();
    setup({ rows: null });
    expect(screen.getByText("No note is open.")).toBeTruthy();
  });
});
