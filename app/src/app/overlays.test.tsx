// The window's menus, prompts, notices and status bar items, wired to the
// workspace and notesd (batch 2's first cards), over the mock.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { fileRow, findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, notices: [], recentCommands: [] });
  usePrefs.getState().setPrefs({ lastVault: null, editingMode: "live" });
  localStorage.clear();
  vi.restoreAllMocks();
});

const files = { 3: { "a.md": "Alpha links [[b]].\n", "Deep/b.md": "Beta\n", "pic.png": "png" } };

async function start() {
  const mock = new MockTransport(mockVaults(), { files });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  await findFileRow("a.md");
  return { mock, user: userEvent.setup() };
}

const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));
const field = (name: string) => screen.getByRole("combobox", { name });
const options = () => within(screen.getByRole("listbox")).getAllByRole("option");

describe("the quick switcher", () => {
  it("opens with Ctrl+O, finds a note by its name, and opens it", async () => {
    const { user } = await start();
    await user.keyboard("{Control>}o{/Control}");
    await user.type(field("Quick switcher"), "beta");
    await user.clear(field("Quick switcher"));
    await user.type(field("Quick switcher"), "b");
    expect(options()[0]?.textContent).toBe("bDeep");
    await user.keyboard("{Enter}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(tabs()).toEqual(["b"]);
  });

  it("lists the recent files with no query, and creates a note named as typed", async () => {
    const { mock, user } = await start();
    await user.click(fileRow("a.md"));
    await user.keyboard("{Control>}o{/Control}");
    expect(options().map((o) => o.textContent)).toEqual(["a"]);
    await user.type(field("Quick switcher"), "Seed swap");
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    await waitFor(() => expect(tabs()).toEqual(["Seed swap"]));
    expect(mock.vault(3).text("Seed swap.md")).toBe("");
  });

  it("opens from the ribbon, closes on Escape, and gives focus back", async () => {
    const { user } = await start();
    const button = screen.getByRole("button", { name: "Open quick switcher" });
    await user.click(button);
    expect(document.activeElement).toBe(field("Quick switcher"));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
  });
});

describe("the command palette", () => {
  it("runs a command, and lists it first next time", async () => {
    const { user } = await start();
    await user.click(screen.getByRole("button", { name: "Open command palette" }));
    await user.type(field("Command palette"), "new tab");
    expect(options()[0]?.textContent).toBe("New tabCtrl + T");
    await user.keyboard("{Enter}");
    expect(tabs()).toEqual(["New tab", "New tab"]);
    expect(JSON.parse(localStorage.getItem("frostnotes.recentCommands") ?? "[]")).toEqual(["workspace:new-tab"]);
    await user.keyboard("{Control>}p{/Control}");
    expect(options()[0]?.textContent).toBe("New tabCtrl + T");
  });

  it("opens the quick switcher from a command, after the palette closed", async () => {
    const { user } = await start();
    await user.keyboard("{Control>}p{/Control}");
    await user.type(field("Command palette"), "quick switcher");
    await user.keyboard("{Enter}");
    expect(screen.getByRole("dialog", { name: "Quick switcher" })).toBeTruthy();
  });
});

describe("the tab menus", () => {
  it("closes a tab from its context menu", async () => {
    const { user } = await start();
    await user.click(fileRow("a.md"));
    fireEvent.click(fileRow("Deep/b.md"), { ctrlKey: true });
    expect(tabs()).toEqual(["a", "b"]);
    fireEvent.contextMenu(screen.getByRole("tab", { name: "a" }));
    const menu = screen.getByRole("menu", { name: "Tab" });
    expect(within(menu).getByRole("menuitem", { name: "Close tabs to the right" }).getAttribute("aria-disabled")).toBe(
      null,
    );
    await user.click(within(menu).getByRole("menuitem", { name: "Close" }));
    expect(tabs()).toEqual(["b"]);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("lists a group's tabs, the active one checked, and activates the one chosen", async () => {
    const { user } = await start();
    await user.click(fileRow("a.md"));
    fireEvent.click(fileRow("Deep/b.md"), { ctrlKey: true });
    await user.click(screen.getByRole("button", { name: "Tab list" }));
    const items = within(screen.getByRole("menu", { name: "Tab list" })).getAllByRole("menuitemcheckbox");
    expect(items.map((i) => [i.getAttribute("aria-label"), i.getAttribute("aria-checked")])).toEqual([
      ["a", "false"],
      ["b", "true"],
    ]);
    await user.click(items[0] as HTMLElement);
    expect(screen.getByRole("tab", { name: "a" }).getAttribute("aria-selected")).toBe("true");
  });
});

describe("More options", () => {
  it("copies the path and says so", async () => {
    const { user } = await start(); // user-event puts its own clipboard in place
    await user.click(fileRow("Deep/b.md"));
    await user.click(screen.getByRole("button", { name: "More options" }));
    await user.click(screen.getByRole("menuitem", { name: "Copy path" }));
    expect(await screen.findByText("Copied the path.")).toBeTruthy();
    expect(await navigator.clipboard.readText()).toBe("Deep/b.md");
  });

  it("deletes the file with its base once the user agrees", async () => {
    const { mock, user } = await start();
    await user.click(fileRow("a.md"));
    for (const label of ["Cancel", "Delete"]) {
      await user.click(screen.getByRole("button", { name: "More options" }));
      await user.click(screen.getByRole("menuitem", { name: "Delete file" }));
      const dialog = await screen.findByRole("alertdialog", { name: 'Delete "a"?' });
      expect(within(dialog).getByText("You can restore it from its version history.")).toBeTruthy();
      if (label === "Cancel") expect(mock.vault(3).text("a.md")).toBe("Alpha links [[b]].\n");
      await user.click(within(dialog).getByRole("button", { name: label }));
    }
    await waitFor(() => expect(mock.vault(3).text("a.md")).toBeUndefined());
    expect(await screen.findByText("Deleted a.")).toBeTruthy();
  });

  it("opens the version history in a new tab", async () => {
    const { user } = await start();
    await user.click(fileRow("a.md"));
    await user.click(screen.getByRole("button", { name: "More options" }));
    await user.click(screen.getByRole("menuitem", { name: "Open version history" }));
    expect(tabs()).toEqual(["a", "History of a"]);
  });
});

describe("the status bar", () => {
  it("counts the active note's backlinks and opens the Backlinks pane", async () => {
    const { user } = await start();
    await user.click(fileRow("Deep/b.md"));
    const count = await screen.findByRole("button", { name: "1 backlink" });
    await user.click(count);
    expect(screen.getByRole("tab", { name: "Backlinks" }).getAttribute("aria-selected")).toBe("true");
  });

  it("switches the editing mode from its menu", async () => {
    const { user } = await start();
    await user.click(fileRow("a.md"));
    await user.click(screen.getByRole("button", { name: "Editing mode: Live Preview" }));
    const menu = screen.getByRole("menu", { name: "Editing mode" });
    expect(within(menu).getByRole("menuitemcheckbox", { name: "Live Preview" }).getAttribute("aria-checked")).toBe(
      "true",
    );
    await user.click(within(menu).getByRole("menuitemcheckbox", { name: "Source mode" }));
    expect(usePrefs.getState().editingMode).toBe("source");
    await user.click(screen.getByRole("button", { name: "Editing mode: Source mode" }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "Reading view" }));
    expect(screen.getByRole("button", { name: "Editing mode: Reading view" })).toBeTruthy();
  });
});

describe("notices", () => {
  it("show what the store says and leave when clicked", async () => {
    await start();
    act(() => {
      useOverlays.setState({ notices: [{ id: "x", text: "Moved a to Deep." }] });
    });
    fireEvent.click(screen.getByText("Moved a to Deep."));
    expect(screen.queryByText("Moved a to Deep.")).toBeNull();
  });
});
