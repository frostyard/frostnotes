import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { StatusBar, type StatusBarProps } from "./StatusBar";

function setup(over: Partial<StatusBarProps> = {}) {
  const props: StatusBarProps = {
    connection: { state: "ready" },
    sync: null,
    backlinks: null,
    counts: null,
    mode: null,
    onConflicts: vi.fn(),
    onBacklinks: vi.fn(),
    onMode: vi.fn(),
    ...over,
  };
  render(<StatusBar {...props} />);
  return props;
}

const bar = () => screen.getByRole("status");

describe("StatusBar", () => {
  it("shows the connection only while it is not ready", () => {
    setup({ connection: { state: "connecting" } });
    expect(bar().textContent).toBe("Connecting to notesd…");
  });

  it("names why it reconnects", () => {
    setup({ connection: { state: "lost", reason: "socket closed" } });
    expect(bar().textContent).toBe("Reconnecting to notesd… (socket closed)");
  });

  it("is empty when ready with nothing to show", () => {
    setup();
    expect(bar().textContent).toBe("");
    expect(bar().className).toContain("bg-toolbar");
  });

  it("shows the sync state as a named icon, and conflicts as a button", () => {
    const props = setup({ sync: { state: "idle", text: "Synced 9:41", conflicts: 2 } });
    const icon = screen.getByRole("img", { name: "Synced 9:41" });
    expect(icon.getAttribute("title")).toBe("Synced 9:41");
    expect(icon.getAttribute("data-sync-state")).toBe("idle");
    fireEvent.click(screen.getByRole("button", { name: "2 conflicts" }));
    expect(props.onConflicts).toHaveBeenCalledTimes(1);
  });

  it("turns the icon while syncing, and hides conflicts when there are none", () => {
    setup({ sync: { state: "syncing", text: "Syncing…", conflicts: 0 } });
    const icon = screen.getByRole("img", { name: "Syncing…" });
    expect(icon.getAttribute("data-sync-state")).toBe("syncing");
    expect(icon.querySelector("svg")?.getAttribute("class")).toContain("animate-spin");
    expect(screen.queryByRole("button", { name: /conflict/ })).toBeNull();
  });

  it("says one conflict and one backlink in the singular", () => {
    setup({ sync: { state: "offline", text: "Offline: no route", conflicts: 1 }, backlinks: 1 });
    expect(screen.getByRole("button", { name: "1 conflict" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "1 backlink" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "Offline: no route" }).getAttribute("data-sync-state")).toBe("offline");
  });

  it("opens the Backlinks pane from its count", () => {
    const props = setup({ backlinks: 12 });
    fireEvent.click(screen.getByRole("button", { name: "12 backlinks" }));
    expect(props.onBacklinks).toHaveBeenCalledTimes(1);
  });

  it("counts words and characters with grouped thousands", () => {
    setup({ counts: { words: 1, characters: 1804 } });
    expect(screen.getByText("1 word")).toBeTruthy();
    expect(screen.getByText("1,804 characters")).toBeTruthy();
  });

  it("orders its items: sync, backlinks, counts, mode", () => {
    setup({
      sync: { state: "idle", text: "Synced", conflicts: 3 },
      backlinks: 2,
      counts: { words: 312, characters: 1804 },
      mode: "live",
    });
    expect(bar().textContent).toBe("3 conflicts2 backlinks312 words1,804 characters");
  });

  it("names the editing mode and opens its menu at the button", () => {
    const props = setup({ mode: "source" });
    const button = screen.getByRole("button", { name: "Editing mode: Source mode" });
    expect(button.getAttribute("aria-haspopup")).toBe("menu");
    fireEvent.click(button);
    expect(props.onMode).toHaveBeenCalledTimes(1);
    const [x, y] = (props.onMode as ReturnType<typeof vi.fn>).mock.calls[0] as [number, number];
    expect(typeof x).toBe("number");
    expect(typeof y).toBe("number");
  });

  it("names the other modes", () => {
    const { unmount } = render(
      <StatusBar
        connection={{ state: "ready" }}
        sync={null}
        backlinks={null}
        counts={null}
        mode="live"
        onConflicts={vi.fn()}
        onBacklinks={vi.fn()}
        onMode={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Editing mode: Live Preview" })).toBeTruthy();
    unmount();
    setup({ mode: "read" });
    expect(screen.getByRole("button", { name: "Editing mode: Reading view" })).toBeTruthy();
  });

  it("shows revoked and reset with the alert icon", () => {
    setup({ sync: { state: "revoked", text: "This device was removed from the hub.", conflicts: 0 } });
    expect(
      screen.getByRole("img", { name: "This device was removed from the hub." }).getAttribute("data-sync-state"),
    ).toBe("revoked");
  });
});
