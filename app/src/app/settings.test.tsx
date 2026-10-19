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
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { fileRow, findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
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

  it("list the hotkeys with F1, and say a vault without a hub does not sync", async () => {
    const { user } = await start();
    await user.keyboard("{F1}");
    const hotkeys = within(settings()).getByRole("region", { name: "Hotkeys" });
    const row = within(hotkeys).getByText("Open quick switcher").closest("[data-setting]") as HTMLElement;
    expect(row.querySelector("kbd")?.textContent).toBe("Ctrl + O");
    await user.click(section("Sync"));
    expect(screen.getByText("This vault does not sync with a hub.")).toBeTruthy();
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
