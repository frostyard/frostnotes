import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type OutgoingLink, outgoingLinks } from "../../lib/outgoing";
import type { Link } from "../../rpc/gen/api";
import { OutgoingPane } from "./OutgoingPane";

const L = (target: string, resolved?: string, extra: Partial<Link> = {}): Link => ({
  kind: "wiki",
  target,
  subpath: "",
  alias: "",
  embed: false,
  key: "",
  line: 1,
  ...(resolved !== undefined ? { resolved } : {}),
  ...extra,
});

describe("outgoingLinks", () => {
  it("lists each target once, resolved ones first, both in the order first named", () => {
    const links = [
      L("Seed swap"),
      L("Orchard plan", "Projects/Orchard plan.md"),
      L("map.png", "Attachments/map.png", { embed: true }),
      L("projects/orchard plan", "Projects/Orchard plan.md", { subpath: "#Goals" }),
      L("seed SWAP"),
      L("Compost"),
    ];
    expect(outgoingLinks(links, "Inbox.md")).toEqual([
      { label: "Orchard plan", path: "Projects/Orchard plan.md", target: "Orchard plan" },
      { label: "map.png", path: "Attachments/map.png", target: "map.png" },
      { label: "Seed swap", path: null, target: "Seed swap" },
      { label: "Compost", path: null, target: "Compost" },
    ]);
  });

  it("leaves out links to the note itself", () => {
    const links = [L("", undefined, { subpath: "#Goals" }), L("Inbox", "Inbox.md"), L("Old.MD", "Old.MD")];
    expect(outgoingLinks(links, "Inbox.md")).toEqual([{ label: "Old", path: "Old.MD", target: "Old.MD" }]);
  });
});

const ROWS: OutgoingLink[] = [
  { label: "Orchard plan", path: "Projects/Orchard plan.md", target: "Orchard plan" },
  { label: "Seed swap", path: null, target: "Seed swap" },
];

describe("OutgoingPane", () => {
  it("lists the targets, unresolved ones dimmed", () => {
    render(<OutgoingPane links={ROWS} onOpen={vi.fn()} />);
    const buttons = within(screen.getByRole("list", { name: "Outgoing links" })).getAllByRole("button");
    expect(buttons.map((b) => b.textContent)).toEqual(["Orchard plan", "Seed swap"]);
    expect(buttons.map((b) => b.getAttribute("title"))).toEqual(["Projects/Orchard plan.md", "Seed swap"]);
    expect(buttons[1]?.className).toContain("text-tertiary");
    expect(buttons[0]?.className).not.toContain("text-tertiary");
  });

  it("opens a target with the click's modifiers, and by middle click", () => {
    const onOpen = vi.fn();
    render(<OutgoingPane links={ROWS} onOpen={onOpen} />);
    fireEvent.click(screen.getByRole("button", { name: "Orchard plan" }));
    fireEvent.click(screen.getByRole("button", { name: "Seed swap" }), { ctrlKey: true, altKey: true });
    fireEvent(
      screen.getByRole("button", { name: "Orchard plan" }),
      new MouseEvent("auxclick", { bubbles: true, button: 1 }),
    );
    expect(onOpen.mock.calls).toEqual([
      [ROWS[0], { ctrl: false, alt: false, middle: false }],
      [ROWS[1], { ctrl: true, alt: true, middle: false }],
      [ROWS[0], { ctrl: false, alt: false, middle: true }],
    ]);
  });

  it("says when there are no links, and when no note is open", () => {
    const { unmount } = render(<OutgoingPane links={[]} onOpen={vi.fn()} />);
    expect(screen.getByText("No links found.")).toBeTruthy();
    unmount();
    render(<OutgoingPane links={null} onOpen={vi.fn()} />);
    expect(screen.getByText("No note is open.")).toBeTruthy();
  });
});
