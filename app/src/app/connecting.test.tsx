// The window while notesd is not running (docs/specs/ui.md#connecting): in
// the Flatpak it says why, from notesd.log's end; anywhere else, only that it
// is reconnecting.
import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { readNotesdLog } from "../data/notesdLog";
import { useNotes } from "../data/stores";
import { Connecting } from "./MainWindow";

vi.mock("../data/notesdLog", () => ({ readNotesdLog: vi.fn() }));

const read = vi.mocked(readNotesdLog);
const cause = () => screen.queryByRole("region", { name: "Why notesd is not running" });

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" } });
  read.mockReset();
});

describe("Connecting", () => {
  it("says why notesd did not start once the connection fails", async () => {
    read.mockResolvedValue({
      path: "/s/frostnotes/notesd.log",
      lastLine: "frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn); see x",
      portalRefused: true,
    });
    render(<Connecting />);
    expect(read).not.toHaveBeenCalled();
    expect(cause()).toBeNull();

    act(() => useNotes.setState({ connection: { state: "lost", reason: "connection refused" } }));
    expect(await screen.findByRole("region", { name: "Why notesd is not running" })).toBeTruthy();
    expect(read).toHaveBeenCalledOnce();
    const text = cause()?.textContent ?? "";
    expect(text).toContain("notesd did not start, because the Flatpak portal did not start it.");
    expect(text).toContain("flatpak override --user --show org.frostyard.Frostnotes");
    expect(text).toContain("The log: /s/frostnotes/notesd.log");
    expect(screen.getByRole("status").textContent).toContain("Reconnecting to notesd… (connection refused)");
  });

  it("shows the cause only while the connection is lost, and reads the log again at the next failure", async () => {
    read.mockResolvedValue({ path: "/s/notesd.log", lastLine: "notesd: another notesd uses /d", portalRefused: false });
    useNotes.setState({ connection: { state: "lost", reason: "connection refused" } });
    render(<Connecting />);
    await screen.findByRole("region", { name: "Why notesd is not running" });
    expect(read).toHaveBeenCalledOnce();

    act(() => useNotes.setState({ connection: { state: "connecting" } })); // a retry
    expect(cause()).toBeNull();
    expect(read).toHaveBeenCalledOnce();

    read.mockResolvedValue({ path: "/s/notesd.log", lastLine: "notesd: a newer line", portalRefused: false });
    act(() => useNotes.setState({ connection: { state: "lost", reason: "connection refused" } }));
    await vi.waitFor(() => expect(cause()?.textContent).toContain("notesd: a newer line"));
    expect(read).toHaveBeenCalledTimes(2);

    act(() => useNotes.setState({ connection: { state: "connecting" } }));
    expect(cause()).toBeNull();
  });

  it("shows another last line as it is", async () => {
    read.mockResolvedValue({ path: "/s/notesd.log", lastLine: "notesd: another notesd uses /d", portalRefused: false });
    useNotes.setState({ connection: { state: "lost", reason: "connection refused" } });
    render(<Connecting />);
    await screen.findByRole("region", { name: "Why notesd is not running" });
    expect(cause()?.textContent).toBe(
      "The last line of notesd's log: notesd: another notesd uses /dThe log: /s/notesd.log",
    );
  });

  it("says nothing more outside the Flatpak", async () => {
    read.mockResolvedValue(null);
    useNotes.setState({ connection: { state: "lost", reason: "connection refused" } });
    render(<Connecting />);
    await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
    expect(cause()).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("Reconnecting to notesd… (connection refused)");
  });
});
