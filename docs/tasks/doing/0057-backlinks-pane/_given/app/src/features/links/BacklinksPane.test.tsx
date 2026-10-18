import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { BacklinksPane, type BacklinksPaneProps, type MentionGroup } from "./BacklinksPane";

const LINKED: MentionGroup[] = [
  {
    path: "Daily/2026-10-17.md",
    title: "2026-10-17",
    mentions: [
      {
        line: 3,
        parts: [
          { text: "Walked the ", match: false },
          { text: "[[Orchard plan]]", match: true },
        ],
      },
      { line: 7, parts: [{ text: "[[Orchard plan#Goals]]", match: true }] },
    ],
  },
  { path: "Welcome.md", title: "Welcome", mentions: [{ line: 1, parts: [{ text: "[[Orchard plan]]", match: true }] }] },
];
const UNLINKED: MentionGroup[] = [
  {
    path: "Inbox.md",
    title: "Inbox",
    mentions: [
      {
        line: null,
        parts: [
          { text: "ask about the ", match: false },
          { text: "orchard plan", match: true },
        ],
      },
    ],
  },
];

function setup(over: Partial<BacklinksPaneProps> = {}) {
  const props: BacklinksPaneProps = { linked: LINKED, unlinked: UNLINKED, onOpen: vi.fn(), ...over };
  const view = render(<BacklinksPane {...props} />);
  return { props, view };
}

const section = (name: string) => screen.getByRole("region", { name });
const header = (name: string) => within(section(name)).getAllByRole("button")[0] as HTMLElement;

describe("BacklinksPane", () => {
  it("shows the linked mentions, open, with their count, and the unlinked ones folded", () => {
    setup();
    expect(header("Linked mentions").textContent).toBe("Linked mentions2");
    expect(header("Linked mentions").getAttribute("aria-expanded")).toBe("true");
    expect(header("Unlinked mentions").textContent).toBe("Unlinked mentions1");
    expect(header("Unlinked mentions").getAttribute("aria-expanded")).toBe("false");
    expect(within(section("Unlinked mentions")).queryByText("Inbox")).toBeNull();
  });

  it("groups mentions by note, with the note's link count and each line, the links marked", () => {
    setup();
    const linked = section("Linked mentions");
    expect(within(linked).getByRole("button", { name: "2026-10-17" }).getAttribute("title")).toBe(
      "Daily/2026-10-17.md",
    );
    const line = within(linked).getByRole("button", { name: "Walked the [[Orchard plan]]" });
    const marks = [...line.querySelectorAll("mark")];
    expect(marks.map((m) => m.textContent)).toEqual(["[[Orchard plan]]"]);
    expect(marks[0]?.className).toContain("bg-highlight");
    // The group's row: its title, then how many links it holds.
    expect(within(linked).getByRole("button", { name: "2026-10-17" }).parentElement?.textContent).toBe("2026-10-172");
  });

  it("opens the source at a line, or a note from its title, with the click's modifiers", () => {
    const { props } = setup();
    fireEvent.click(screen.getByRole("button", { name: "[[Orchard plan#Goals]]" }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Welcome" }));
    expect(props.onOpen).toHaveBeenCalledWith("Daily/2026-10-17.md", 7, { ctrl: true, alt: false, middle: false });
    expect(props.onOpen).toHaveBeenCalledWith("Welcome.md", null, { ctrl: false, alt: false, middle: false });
  });

  it("folds a note's group and a whole section", () => {
    setup();
    fireEvent.click(screen.getByRole("button", { name: "Fold 2026-10-17" }));
    expect(screen.queryByRole("button", { name: "Walked the [[Orchard plan]]" })).toBeNull();
    expect(screen.getByRole("button", { name: "Unfold 2026-10-17" }).getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(header("Linked mentions"));
    expect(screen.queryByRole("button", { name: "Welcome" })).toBeNull();
  });

  it("opens the unlinked mentions, whose lines open their note", () => {
    const { props } = setup();
    fireEvent.click(header("Unlinked mentions"));
    fireEvent.click(screen.getByRole("button", { name: "ask about the orchard plan" }));
    expect(props.onOpen).toHaveBeenCalledWith("Inbox.md", null, { ctrl: false, alt: false, middle: false });
  });

  it("says when there are none, shows no count while the unlinked load, and when no note is open", () => {
    const { view } = setup({ linked: [], unlinked: null });
    expect(screen.getByText("No backlinks found.")).toBeTruthy();
    expect(header("Unlinked mentions").textContent).toBe("Unlinked mentions");
    fireEvent.click(header("Unlinked mentions"));
    expect(screen.queryByText("No unlinked mentions found.")).toBeNull();
    view.unmount();
    const second = setup({ unlinked: [] });
    fireEvent.click(header("Unlinked mentions"));
    expect(screen.getByText("No unlinked mentions found.")).toBeTruthy();
    second.view.unmount();
    setup({ linked: null });
    expect(screen.getByText("No note is open.")).toBeTruthy();
  });
});
