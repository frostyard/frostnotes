// Settings, the history tab and moving files, wired to the stores and
// notesd (batch 2's last cards), over the mock.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { DEFAULT_PREFS, usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import type { SyncState, SyncStatus } from "../rpc/gen/api";
import { HUB_URL, MockTransport, mockVaults } from "../rpc/mock/mock";
import { fileRow, findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";
import { settingsSections } from "./Settings";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null, joinHold: false });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, move: null, confirm: null, settings: null, notices: [] });
  usePrefs.getState().setPrefs({ ...DEFAULT_PREFS, lastVault: null });
  delete document.documentElement.dataset.theme;
  localStorage.clear();
});

const FILES = { 3: { "a.md": "Alpha.\n", "Deep/b.md": "Beta\n" } };

async function start() {
  const mock = new MockTransport(mockVaults(), { files: FILES });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  await findFileRow("a.md");
  await waitFor(() => expect(useVaultState.getState().settings).not.toBeNull());
  return { mock, user: userEvent.setup() };
}

const settings = () => screen.getByRole("dialog", { name: "Settings" });
const section = (name: string) => within(settings()).getByRole("button", { name });
const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));

describe("the settings", () => {
  it("open with Ctrl+, at the editor's, and change a preference at once", async () => {
    const { user } = await start();
    await user.keyboard("{Control>},{/Control}");
    expect(within(settings()).getByRole("region", { name: "Editor" })).toBeTruthy();
    await user.click(screen.getByRole("switch", { name: "Readable line length" }));
    expect(usePrefs.getState().readableLineLength).toBe(false);
    expect(screen.getByRole("switch", { name: "Readable line length" }).getAttribute("aria-checked")).toBe("false");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Settings" })).toBeNull();
  });

  it("set the theme and font size on the page", async () => {
    const { user } = await start();
    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(section("Appearance"));
    await user.selectOptions(screen.getByRole("combobox", { name: "Theme" }), "dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    const size = screen.getByRole("spinbutton", { name: "Font size" });
    await user.clear(size);
    await user.type(size, "20{Enter}");
    expect(usePrefs.getState().fontSize).toBe(20);
    expect(document.documentElement.style.getPropertyValue("--note-font-size")).toBe("20px");
    await user.selectOptions(screen.getByRole("combobox", { name: "Theme" }), "system");
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });

  it("configure the vault's settings through notesd", async () => {
    const { user } = await start();
    await user.keyboard("{Control>},{/Control}");
    await user.click(section("Files and links"));
    expect(screen.getByRole("textbox", { name: "Folder to create new notes in" }).hasAttribute("disabled")).toBe(true);
    await user.selectOptions(screen.getByRole("combobox", { name: "Default location for new notes" }), "folder");
    await waitFor(() => expect(useVaultState.getState().settings?.newNoteLocation).toBe("folder"));
    const folder = screen.getByRole("textbox", { name: "Folder to create new notes in" });
    expect(folder.hasAttribute("disabled")).toBe(false);
    await user.type(folder, "Inbox{Enter}");
    await waitFor(() => expect(useVaultState.getState().settings?.newNoteFolder).toBe("Inbox"));
    await user.click(screen.getByRole("switch", { name: "Automatically update internal links" }));
    await waitFor(() => expect(useVaultState.getState().settings?.updateLinks).toBe(true));
  });

  it("list the hotkeys with F1, and offer to join a hub", async () => {
    const { user } = await start();
    await user.keyboard("{F1}");
    const hotkeys = within(settings()).getByRole("region", { name: "Hotkeys" });
    const row = within(hotkeys).getByText("Open quick switcher").closest("[data-setting]") as HTMLElement;
    expect(row.querySelector("kbd")?.textContent).toBe("Ctrl + O");
    await user.click(section("Sync"));
    await waitFor(() => expect(useVaultState.getState().syncAvailable).toBe(true));
    expect(screen.getByText("Sync with a hub")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Join… Sync with a hub" })).toBeTruthy();
    expect(screen.queryByText("This vault does not sync with a hub.")).toBeNull();
  });
});

describe("the Sync section", () => {
  it("joins a hub for the window's vault, and leaves it after asking", async () => {
    const { mock, user } = await start();
    await user.keyboard("{Control>},{/Control}");
    await user.click(section("Sync"));
    await waitFor(() => expect(useVaultState.getState().syncAvailable).toBe(true));
    await user.click(screen.getByRole("button", { name: "Join… Sync with a hub" }));
    const sheet = within(await screen.findByRole("dialog", { name: "Join a hub" }));
    await user.type(sheet.getByRole("textbox", { name: "Hub URL" }), "https://hub.example/");
    await user.type(sheet.getByRole("textbox", { name: "Hub vault name" }), "shared");
    await user.click(sheet.getByRole("button", { name: "Join" }));
    await waitFor(() => expect(useVaultState.getState().sync?.hubVault).toBe("shared"));
    expect(mock.calls.find((c) => c.method === "sync.join")?.params).toEqual({
      vaultId: 3,
      url: "https://hub.example",
      name: "shared",
    });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Join a hub" })).toBeNull());
    expect(screen.getByText("https://hub.example")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Leave Stop syncing" }));
    const ask = await screen.findByRole("alertdialog", { name: 'Stop syncing "garden"?' });
    expect(ask.textContent).toContain("This vault's files stay on this device, and the hub keeps its copy.");
    await user.click(within(ask).getByRole("button", { name: "Leave" }));
    await waitFor(() => expect(useVaultState.getState().sync).toBeNull());
    expect(mock.calls.find((c) => c.method === "sync.leave")?.params).toEqual({ vaultId: 3 });
    await waitFor(() => expect(screen.getByText("Sync with a hub")).toBeTruthy());
  });

  it("pairs again with a new code for a vault whose hub refused the token", async () => {
    const mock = new MockTransport(mockVaults(), {
      files: FILES,
      synced: [{ vaultId: 3, url: HUB_URL, name: "shared", state: "revoked" }],
    });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    await waitFor(() => expect(useVaultState.getState().sync?.state).toBe("revoked"));
    const user = userEvent.setup();
    act(() => useOverlays.setState({ settings: "sync" }));
    const region = await within(settings()).findByRole("region", { name: "Sync" });
    expect(within(region).getByText(HUB_URL)).toBeTruthy();
    expect(within(region).getByText("Pair again")).toBeTruthy();
    await user.click(within(region).getByRole("button", { name: "Pair again… Pair again" }));
    const sheet = within(await screen.findByRole("dialog", { name: "Join a hub" }));
    expect(sheet.queryByRole("textbox", { name: "Hub vault name" })).toBeNull();
    const url = sheet.getByRole("textbox", { name: "Hub URL" }) as HTMLInputElement;
    expect(url.value).toBe(HUB_URL);
    await user.click(sheet.getByRole("button", { name: "Join" }));
    await waitFor(() => expect(mock.calls.some((c) => c.method === "sync.pair")).toBe(true));
    expect(mock.calls.find((c) => c.method === "sync.pair")?.params).toEqual({ url: HUB_URL });
    expect(mock.calls.some((c) => c.method === "sync.join")).toBe(false);
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Join a hub" })).toBeNull());
  });

  it("offers no join to a browser inside lodge", async () => {
    const mock = new MockTransport(mockVaults(), { files: FILES, caller: "session", node: "phone" });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    await waitFor(() => expect(useVaultState.getState().syncAvailable).toBe(false));
    act(() => useOverlays.setState({ settings: "sync" }));
    const region = await within(settings()).findByRole("region", { name: "Sync" });
    expect(within(region).getByText("This vault does not sync with a hub.")).toBeTruthy();
    expect(within(region).queryByRole("button", { name: /Join|Leave|Pair again/ })).toBeNull();
  });
});

describe("the Sync section's rows", () => {
  const status = (state: SyncState): SyncStatus => ({
    vaultId: 3,
    url: HUB_URL,
    hubVault: "shared",
    state,
    cursor: 0,
    hubSeq: 0,
    pending: 0,
    conflicts: 1,
    refused: [],
    skipped: [],
  });
  const syncRows = (sync: SyncStatus | null, canJoin: boolean) =>
    settingsSections(DEFAULT_PREFS, null, sync, null, canJoin).find((s) => s.id === "sync")?.rows ?? [];
  const ids = (sync: SyncStatus | null, canJoin: boolean) => syncRows(sync, canJoin).map((r) => r.id);

  it("offers Join for a vault with no hub, and nothing to a caller that may not join", () => {
    expect(ids(null, true)).toEqual(["sync:join"]);
    expect(ids(null, false)).toEqual(["sync:none"]);
  });

  it("says what Join and Pair again do", () => {
    const join = syncRows(null, true).find((r) => r.id === "sync:join");
    expect(join?.description).toBe("Join a vault on a hub. Notes then sync on every device that joins it.");
    const pair = syncRows(status("revoked"), true).find((r) => r.id === "sync:pair");
    expect(pair?.description).toBe("The hub refused this device's token. Pair again with a new code to sync again.");
  });

  it("offers Leave for a synced vault, and Pair again where the hub refused the token", () => {
    expect(ids(status("idle"), true)).toContain("sync:leave");
    expect(ids(status("idle"), true)).not.toContain("sync:pair");
    expect(ids(status("revoked"), true)).toContain("sync:pair");
    expect(ids(status("revoked"), true)).toContain("sync:leave");
    expect(ids(status("revoked"), false)).not.toContain("sync:pair");
    expect(ids(status("revoked"), false)).not.toContain("sync:leave");
  });
});

describe("the Hub section", () => {
  it("lists a hub's devices and pending pairings, and revokes a device after asking", async () => {
    const mock = new MockTransport(mockVaults(), {
      files: FILES,
      caller: "session",
      node: "phone",
      hub: {
        status: {
          hub: "0123456789abcdef0123456789abcdef",
          url: "https://lodge.example.ts.net/device/notes",
          vaults: 1,
          devices: 1,
          connected: 1,
        },
        devices: [
          {
            id: 4,
            name: "laptop",
            pairedAt: "2026-10-01T09:00:00Z",
            seenAt: "2026-10-18T22:00:00Z",
            connected: true,
            revoked: false,
          },
        ],
        pending: [
          {
            code: "7K4M-Q2XD",
            name: "desk",
            addr: "desk (ana@example.com)",
            createdAt: "2026-10-19T03:00:00Z",
            expiresAt: "2026-10-19T03:10:00Z",
            approved: false,
          },
        ],
      },
    });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    act(() => useOverlays.setState({ settings: "hub" }));
    const hub = await within(settings()).findByRole("region", { name: "Hub" });
    expect(within(hub).getByText("https://lodge.example.ts.net/device/notes")).toBeTruthy();
    expect(within(hub).getByText("notesctl hub approve 7K4M-Q2XD")).toBeTruthy();
    expect(within(hub).getByText(/From desk \(ana@example.com\)/)).toBeTruthy();
    await user.click(within(hub).getByRole("button", { name: "Revoke laptop" }));
    await user.click(await screen.findByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(within(settings()).getByText("Revoked")).toBeTruthy());
    expect(mock.calls.some((c) => c.method === "hub.revoke")).toBe(true);
  });

  it("is absent where notesd is no hub", async () => {
    await start();
    act(() => useOverlays.setState({ settings: "general" }));
    await waitFor(() => expect(settings()).toBeTruthy());
    expect(within(settings()).queryByRole("button", { name: "Hub" })).toBeNull();
  });
});

describe("the history tab", () => {
  it("lists the versions, shows one and its changes, restores it, and compares it", async () => {
    const { mock, user } = await start();
    act(() => mock.vault(3).outside("a.md", "Alpha, rewritten.\n"));
    await act(async () => useWorkspace.getState().update((w) => open(w, { type: "history", path: "a.md" }, "tab")));
    const versions = await screen.findByRole("navigation", { name: "Versions" });
    await waitFor(() => expect(within(versions).getAllByRole("button")).toHaveLength(2));
    await user.click(within(versions).getAllByRole("button")[1] as HTMLElement);
    await waitFor(() => expect(screen.getByTestId("version-text").textContent).toContain("Alpha."));
    await user.click(screen.getByRole("button", { name: "Show changes" }));
    expect(screen.getByRole("button", { name: "Hide changes" }).getAttribute("aria-pressed")).toBe("true");
    await waitFor(() => expect(screen.getByTestId("version-text").querySelector(".cm-deletedChunk")).not.toBeNull());
    await user.click(screen.getByRole("button", { name: "Restore this version" }));
    await waitFor(() => expect(mock.vault(3).text("a.md")).toBe("Alpha.\n"));
    expect(await screen.findByText("Restored the version.")).toBeTruthy();
    await waitFor(() => expect(within(versions).getAllByRole("button")).toHaveLength(3));
    await user.click(within(versions).getAllByRole("button")[2] as HTMLElement);
    await user.click(screen.getByRole("button", { name: "Compare" }));
    expect(tabs()).toEqual(["New tab", "History of a", "Merge a"]);
  });
});

describe("moving a file", () => {
  it("moves the active file to a folder chosen from More options", async () => {
    const { mock, user } = await start();
    await user.click(fileRow("a.md"));
    await user.click(screen.getByRole("button", { name: "More options" }));
    await user.click(screen.getByRole("menuitem", { name: "Move file to…" }));
    const field = screen.getByRole("combobox", { name: "Move to folder" });
    expect(field.getAttribute("placeholder")).toBe('Move "a" to…');
    await user.type(field, "deep");
    fireEvent.keyDown(field, { key: "Enter" });
    const dialog = await screen.findByRole("alertdialog", { name: "Update links?" });
    await user.click(within(dialog).getByRole("button", { name: "Don't update" }));
    await waitFor(() => expect(mock.vault(3).text("Deep/a.md")).toBe("Alpha.\n"));
    await waitFor(() => expect(tabs()).toEqual(["a"]));
    expect(mock.vault(3).text("a.md")).toBeUndefined();
  });

  it("moves a folder from its menu, never into itself", async () => {
    const { mock, user } = await start();
    fireEvent.contextMenu(fileRow("Deep"));
    await user.click(screen.getByRole("menuitem", { name: "Move folder to…" }));
    const options = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(options).toEqual(["/"]);
    fireEvent.keyDown(screen.getByRole("combobox", { name: "Move to folder" }), { key: "Escape" });
    expect(mock.vault(3).text("Deep/b.md")).toBe("Beta\n");
  });
});
