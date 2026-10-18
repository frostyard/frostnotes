import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { sizeText, whenText } from "../../lib/when";
import { type HistoryEntry, HistoryList, type HistoryListProps, SOURCE_ICONS } from "./HistoryList";

const NOW = new Date(2026, 9, 18, 15, 30); // Oct 18, 2026, 15:30 local time

describe("whenText", () => {
  it("says today and yesterday by name, other days by date, with a 24-hour clock", () => {
    expect(whenText(new Date(2026, 9, 18, 9, 41), NOW)).toBe("Today at 9:41");
    expect(whenText(new Date(2026, 9, 18, 0, 5), NOW)).toBe("Today at 0:05");
    expect(whenText(new Date(2026, 9, 17, 21, 5), NOW)).toBe("Yesterday at 21:05");
    expect(whenText(new Date(2026, 9, 15, 9, 41), NOW)).toBe("Oct 15, 2026 at 9:41");
    expect(whenText(new Date(2025, 9, 18, 9, 41), NOW)).toBe("Oct 18, 2025 at 9:41");
  });

  it("knows yesterday across a month's and a year's turn", () => {
    expect(whenText(new Date(2026, 8, 30, 8, 0), new Date(2026, 9, 1, 8, 0))).toBe("Yesterday at 8:00");
    expect(whenText(new Date(2025, 11, 31, 23, 59), new Date(2026, 0, 1, 0, 1))).toBe("Yesterday at 23:59");
  });
});

describe("sizeText", () => {
  it("writes bytes, KB and MB, with one decimal below ten", () => {
    expect(sizeText(1)).toBe("1 byte");
    expect(sizeText(0)).toBe("0 bytes");
    expect(sizeText(1023)).toBe("1023 bytes");
    expect(sizeText(1024)).toBe("1 KB");
    expect(sizeText(1536)).toBe("1.5 KB");
    expect(sizeText(12 * 1024 + 300)).toBe("12 KB");
    expect(sizeText(2.34 * 1024 * 1024)).toBe("2.3 MB");
    expect(sizeText(40 * 1024 * 1024)).toBe("40 MB");
  });
});

const V = (id: string, at: Date, extra: Partial<HistoryEntry> = {}): HistoryEntry => ({
  id,
  at: at.toISOString(),
  source: "editor",
  author: "frostnotes-app",
  size: 1536,
  conflict: false,
  deleted: false,
  ...extra,
});

const VERSIONS = [
  V("v3", new Date(2026, 9, 18, 9, 41)),
  V("v2", new Date(2026, 9, 17, 21, 5), { source: "agent", author: "claude-code", conflict: true, size: 300 }),
  V("v1", new Date(2026, 9, 15, 9, 41), { source: "outside", author: "", size: null }),
  V("v0", new Date(2026, 9, 14, 8, 0), { deleted: true }),
];
const REVISIONS = [V("r7", new Date(2026, 9, 18, 10, 0), { source: "sync", author: "laptop", size: 2048 })];

function setup(over: Partial<HistoryListProps> = {}) {
  const props: HistoryListProps = {
    versions: VERSIONS,
    revisions: null,
    selected: null,
    onSelect: vi.fn(),
    now: NOW,
    ...over,
  };
  const view = render(<HistoryList {...props} />);
  return { props, view };
}

const rows = (within_: HTMLElement) => within(within_).getAllByRole("button");

describe("HistoryList", () => {
  it("lists the versions, each with its time, source, author and size", () => {
    setup();
    const nav = screen.getByRole("navigation", { name: "Versions" });
    expect(rows(nav).map((r) => r.textContent)).toEqual([
      "Today at 9:41frostnotes-app · 1.5 KB",
      "Yesterday at 21:05Conflictclaude-code · 300 bytes",
      "Oct 15, 2026 at 9:41",
      "Oct 14, 2026 at 8:00Deleted",
    ]);
    expect(rows(nav).map((r) => r.querySelector("[data-source]")?.getAttribute("data-source"))).toEqual([
      "editor",
      "agent",
      "outside",
      "editor",
    ]);
    const badge = within(rows(nav)[1] as HTMLElement).getByText("Conflict");
    expect(badge.className).toContain("bg-badge");
  });

  it("has an icon for each source", () => {
    expect(Object.keys(SOURCE_ICONS).sort()).toEqual(["agent", "editor", "outside", "sync"]);
  });

  it("marks the selected row and selects a clicked one", () => {
    const { props } = setup({ selected: "v2" });
    const nav = screen.getByRole("navigation", { name: "Versions" });
    expect(rows(nav).map((r) => r.getAttribute("aria-current"))).toEqual([null, "true", null, null]);
    expect(rows(nav)[1]?.className).toContain("bg-selection-sidebar");
    fireEvent.click(rows(nav)[2] as HTMLElement);
    expect(props.onSelect).toHaveBeenCalledWith("v1");
  });

  it("lists the hub's revisions under On the hub for a synced vault", () => {
    const { props } = setup({ revisions: REVISIONS });
    const hub = screen.getByRole("region", { name: "On the hub" });
    expect(within(hub).getByRole("heading", { name: "On the hub" })).toBeTruthy();
    expect(rows(hub).map((r) => r.textContent)).toEqual(["Today at 10:00laptop · 2 KB"]);
    fireEvent.click(rows(hub)[0] as HTMLElement);
    expect(props.onSelect).toHaveBeenCalledWith("r7");
  });

  it("says when there are none, and leaves the hub out for a vault that does not sync", () => {
    const { view } = setup({ versions: [], revisions: [] });
    expect(screen.getByText("No versions yet.")).toBeTruthy();
    expect(screen.getByText("No revisions on the hub.")).toBeTruthy();
    view.unmount();
    setup();
    expect(screen.queryByRole("region", { name: "On the hub" })).toBeNull();
  });
});
