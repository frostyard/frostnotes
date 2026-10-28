// After the switch changes, the shell asks the desktop again, and the
// Desktop permission row says that it asks until a new answer is in
// (ADR-0033). The window reads the answer every second, so this test waits
// with real timers.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { DEFAULT_PREFS, usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { emptyWorkspace } from "../workspace/model";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

const { readBackground, setBackground } = vi.hoisted(() => ({ readBackground: vi.fn(), setBackground: vi.fn() }));

vi.mock("../data/background", () => ({ readBackground, setBackground }));

const answer = (autostart: boolean, at: string) => ({
  keepRunning: true,
  portal: { at, autostartRequested: true, response: 0, background: true, autostart, error: null },
});

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, move: null, confirm: null, settings: null, notices: [] });
  usePrefs.getState().setPrefs({ ...DEFAULT_PREFS, lastVault: null });
  localStorage.clear();
});

it("says that it asks the desktop until the new answer arrives", async () => {
  const before = answer(true, "2026-10-28T09:00:00Z");
  const after = answer(false, "2026-10-28T09:00:05Z");
  readBackground.mockResolvedValue(before);
  setBackground.mockResolvedValue(before);
  const mock = new MockTransport(mockVaults(), { files: { 3: { "a.md": "Alpha.\n" } } });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  act(() => useOverlays.setState({ settings: "background" }));
  const dialog = await screen.findByRole("dialog", { name: "Settings" });
  const section = await within(dialog).findByRole("region", { name: "Background" });
  expect(within(section).getByText("Allowed; notesd starts at login.")).toBeTruthy();

  readBackground.mockResolvedValue(after);
  fireEvent.click(within(section).getByRole("switch", { name: "Keep syncing when no window is open" }));
  expect(setBackground).toHaveBeenCalledWith(false);
  expect(within(section).getByText("Asking the desktop…")).toBeTruthy();
  await waitFor(() => expect(within(section).getByText("Allowed; notesd does not start at login.")).toBeTruthy());
});
