// The Desktop permission row follows the desktop's answer while the settings
// are open (ADR-0033): the window reads the answer every 2 s, with no limit,
// and the row says "Asking the desktop…" until an answer read the setting
// after the change. An answer's generation is the shell's counter as its
// request read it; the change's generation is the counter as the change left
// it, so an answer at or above it is about the new setting, from any helper.
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
const NO_ANSWER = "Not known: the desktop did not answer (no session bus).";
/** GEN is the change's number, its generation; the answers count around it. */
const GEN = 7;

/** portal is the portal's answer, as the helper wrote it. */
const portal = (generation: number | null, autostart: boolean): PortalAnswer => ({
  at: "2026-10-28T09:00:00Z",
  autostartRequested: true,
  response: 0,
  background: true,
  autostart,
  error: null,
  generation,
});

/** failed is the answer of a request that never reached the portal. */
const failed = (generation: number | null): PortalAnswer => ({
  at: "2026-10-28T09:00:00Z",
  autostartRequested: true,
  response: null,
  background: null,
  autostart: null,
  error: "no session bus",
  generation,
});

/** background is the shell's answer, with the change's generation when it made one. */
const background = (keepRunning: boolean, p: PortalAnswer | null, generation: number | null = null): Background => ({
  keepRunning,
  portal: p,
  generation,
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

/** toggle turns the setting off, and answers the change's generation. */
async function toggle(section: HTMLElement, generation: number | null) {
  setBackground.mockResolvedValue(background(false, portal(GEN - 1, true), generation));
  fireEvent.click(within(section).getByRole("switch", { name: "Keep syncing when no window is open" }));
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

it("ends the wait on an answer of its own change", async () => {
  const section = await ready(background(true, portal(GEN - 1, true)));
  await toggle(section, GEN);
  expect(within(section).getByText(ASKING)).toBeTruthy();

  readBackground.mockResolvedValue(background(false, portal(GEN, false)));
  await tick(POLL);
  expect(within(section).queryByText(ASKING)).toBeNull();
  expect(within(section).getByText(NO_LOGIN)).toBeTruthy();
});

it("ends the wait on a failure of its own change, and shows its error", async () => {
  const section = await ready(background(true, portal(GEN - 1, true)));
  await toggle(section, GEN);
  expect(within(section).getByText(ASKING)).toBeTruthy();

  readBackground.mockResolvedValue(background(false, failed(GEN)));
  await tick(POLL);
  expect(within(section).queryByText(ASKING)).toBeNull();
  expect(within(section).getByText(NO_ANSWER)).toBeTruthy();
});

it("keeps asking while the answer counts an older change", async () => {
  const section = await ready(background(true, portal(GEN - 1, true)));
  await toggle(section, GEN);

  readBackground.mockResolvedValue(background(false, portal(GEN - 1, false)));
  await tick(POLL);
  expect(within(section).getByText(ASKING)).toBeTruthy();
  await tick(POLL);
  expect(within(section).getByText(ASKING)).toBeTruthy();
});

it("ends the wait on a later helper's answer, however late it comes", async () => {
  const before = background(true, portal(GEN - 1, true));
  const section = await ready(before);
  await toggle(section, GEN);
  let reads = 0;
  readBackground.mockReset();
  readBackground.mockImplementation(async () => {
    reads += 1;
    return reads > 30 ? background(false, portal(GEN + 1, false)) : background(false, before.portal);
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
