// The Desktop permission row follows the desktop's answer while the settings
// are open (ADR-0033): the window reads the answer every 2 s, with no limit,
// and the row says "Asking the desktop…" until an answer was read after the
// change. An answer's askedMs says when its request read the setting; the
// change's since says when the setting was written, so an answer from any
// helper that read the setting after it ends the wait.
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { Background, PortalAnswer } from "../data/background";
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

const POLL = 2000;
const ASKING = "Asking the desktop…";
const AT_LOGIN = "Allowed; notesd starts at login.";
const NO_LOGIN = "Allowed; notesd does not start at login.";
/** NOW is the change's moment, its since; the answers are clocked around it. */
const NOW = 1_784_000_000_000;

/** portal is the portal's answer, as the helper wrote it. */
const portal = (askedMs: number | null, autostart: boolean): PortalAnswer => ({
  at: "2026-10-28T09:00:00Z",
  autostartRequested: true,
  response: 0,
  background: true,
  autostart,
  error: null,
  askedMs,
});

/** background is the shell's answer, with the change's since when it made one. */
const background = (keepRunning: boolean, p: PortalAnswer | null, since: number | null = null): Background => ({
  keepRunning,
  portal: p,
  since,
});

/** tick runs the app's timers, and the promises they start, for ms. */
async function tick(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

/**
 * ready renders the app, opens the settings at the Background section whose
 * first answer is before, and moves to fake timers for `setTimeout` only, so
 * that the test drives the 2 s reads itself.
 */
async function ready(before: Background) {
  readBackground.mockReset();
  setBackground.mockReset();
  readBackground.mockResolvedValue(before);
  const mock = new MockTransport(mockVaults(), { files: { 3: { "a.md": "Alpha.\n" } } });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  act(() => useOverlays.setState({ settings: "background" }));
  const dialog = await screen.findByRole("dialog", { name: "Settings" });
  await within(dialog).findByRole("region", { name: "Background" });
  // The open dialog scheduled its reads with real timers: close it, take the
  // test's clock, and open it again, so every read is the test's.
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  act(() => useOverlays.setState({ settings: null }));
  act(() => useOverlays.setState({ settings: "background" }));
  await act(async () => {});
  await act(async () => {});
  const reopened = screen.getByRole("dialog", { name: "Settings" });
  return within(reopened).getByRole("region", { name: "Background" });
}

const switchOf = (section: HTMLElement) =>
  within(section).getByRole("switch", { name: "Keep syncing when no window is open" });

/** toggle turns the setting off, and answers the change's since. */
async function toggle(section: HTMLElement, since: number | null) {
  setBackground.mockResolvedValue(background(false, portal(NOW - 1000, true), since));
  fireEvent.click(switchOf(section));
  await act(async () => {});
}

afterEach(() => {
  vi.useRealTimers();
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, move: null, confirm: null, settings: null, notices: [] });
  usePrefs.getState().setPrefs({ ...DEFAULT_PREFS, lastVault: null });
  localStorage.clear();
});

it("ends the wait when a later helper's answer is in before the next read", async () => {
  const section = await ready(background(true, portal(NOW - 1000, true)));
  await toggle(section, NOW);
  expect(within(section).getByText(ASKING)).toBeTruthy();

  // Another helper read the setting after the change, and its answer is the
  // one in the file: it says what the setting is now.
  readBackground.mockResolvedValue(background(false, portal(NOW + 5, false)));
  await tick(POLL);
  expect(within(section).queryByText(ASKING)).toBeNull();
  expect(within(section).getByText(NO_LOGIN)).toBeTruthy();
});

it("keeps asking while the answer read the setting before the change", async () => {
  const section = await ready(background(true, portal(NOW - 1000, true)));
  await toggle(section, NOW);

  readBackground.mockResolvedValue(background(false, portal(NOW - 5, false)));
  await tick(POLL);
  expect(within(section).getByText(ASKING)).toBeTruthy();
});

it("keeps asking while the answer has no moment", async () => {
  const section = await ready(background(true, portal(NOW - 1000, true)));
  await toggle(section, NOW);

  readBackground.mockResolvedValue(background(false, portal(null, false)));
  await tick(POLL);
  expect(within(section).getByText(ASKING)).toBeTruthy();
});

it("keeps asking until the answer comes after 60 s", async () => {
  const before = background(true, portal(NOW - 1000, true));
  const section = await ready(before);
  await toggle(section, NOW);
  let reads = 0;
  readBackground.mockReset();
  readBackground.mockImplementation(async () => {
    reads += 1;
    return reads > 30 ? background(false, portal(NOW + 7, false)) : background(false, before.portal);
  });

  // 30 reads, one per 2 s, and the old answer still shows "Asking…".
  await tick(30 * POLL);
  expect(reads).toBe(30);
  expect(within(section).getByText(ASKING)).toBeTruthy();

  await tick(POLL);
  expect(reads).toBe(31);
  expect(within(section).queryByText(ASKING)).toBeNull();
  expect(within(section).getByText(NO_LOGIN)).toBeTruthy();
});

it("shows the launcher's first answer when it comes after the dialog opened", async () => {
  const section = await ready(background(true, null));
  expect(within(section).getByText("Not asked yet.")).toBeTruthy();

  readBackground.mockResolvedValue(background(true, portal(null, true)));
  await tick(POLL);
  expect(within(section).getByText(AT_LOGIN)).toBeTruthy();
});
