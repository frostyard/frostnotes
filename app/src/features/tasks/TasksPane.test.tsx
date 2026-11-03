// The Tasks pane groups open tasks and opens a task or its note.
// It uses the mock callbacks for both task and title clicks.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type TaskGroup, TasksPane, type TasksPaneProps } from "./TasksPane";

const GROUPS: TaskGroup[] = [
  {
    path: "Projects/Orchard plan.md",
    title: "Orchard plan",
    tasks: [
      { line: 11, text: "Prune the old pear" },
      { line: 14, text: "Water the quince" },
    ],
  },
  { path: "Inbox.md", title: "Inbox", tasks: [{ line: 3, text: "Answer the letter" }] },
];

function setup(over: Partial<TasksPaneProps> = {}) {
  const props: TasksPaneProps = { groups: GROUPS, onOpen: vi.fn(), ...over };
  const view = render(<TasksPane {...props} />);
  return { props, view };
}

describe("TasksPane", () => {
  it("groups tasks by note with their counts", () => {
    setup();
    const list = screen.getByRole("list", { name: "Tasks" });
    expect(within(list).getByRole("button", { name: "Orchard plan" }).parentElement?.textContent).toBe("Orchard plan2");
    expect(within(list).getByRole("button", { name: "Inbox" }).parentElement?.textContent).toBe("Inbox1");
    screen.getByRole("button", { name: "Prune the old pear" });
  });

  it("shows the empty state", () => {
    setup({ groups: [] });
    expect(screen.getByText("No open tasks.")).toBeTruthy();
  });

  it("opens a task at its line and a title without a line", () => {
    const { props } = setup();
    fireEvent.click(screen.getByRole("button", { name: "Prune the old pear" }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Orchard plan" }), { altKey: true });
    expect(props.onOpen).toHaveBeenCalledWith("Projects/Orchard plan.md", 11, {
      ctrl: true,
      alt: false,
      middle: false,
    });
    expect(props.onOpen).toHaveBeenCalledWith("Projects/Orchard plan.md", null, {
      ctrl: false,
      alt: true,
      middle: false,
    });
  });

  it("folds a note's tasks", () => {
    setup();
    fireEvent.click(screen.getByRole("button", { name: "Fold Orchard plan" }));
    expect(screen.queryByRole("button", { name: "Prune the old pear" })).toBeNull();
    expect(screen.getByRole("button", { name: "Unfold Orchard plan" }).getAttribute("aria-expanded")).toBe("false");
  });

  it("draws no pane while loading", () => {
    setup({ groups: null });
    expect(screen.queryByRole("list", { name: "Tasks" })).toBeNull();
  });
});
