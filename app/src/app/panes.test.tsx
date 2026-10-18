// The sidebar panes wired to the workspace and notesd (batch 2's panes),
// over the mock.
import { EditorView } from "@codemirror/view";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePanes } from "../data/panes";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { DRAG_TYPE } from "../features/explorer/FilesPane";
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
  usePanes.setState({
    searchQuery: "",
    searchFocus: 0,
    tagSort: "name",
    propertySort: "name",
    renaming: null,
    folded: {},
  });
  usePrefs.getState().setPrefs({ lastVault: null });
  localStorage.clear();
  vi.restoreAllMocks();
});

const FILES = {
  3: {
    "Orchard.md": "---\nstatus: active\ntags: [garden]\n---\n# Orchard\n\nPlant the [[Quince]] soon, and a [[Pear]].\n",
    "Quince.md": "# Quince\n\n## Pruning\n\nCut in winter. #garden/fruit\n",
    "Notes.md": "Ask about quince prices.\n",
    "Deep/Seeds.md": "Beans.\n",
  },
};

async function start() {
  const mock = new MockTransport(mockVaults(), { files: FILES });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  await findFileRow("Orchard.md");
  return { mock, user: userEvent.setup() };
}

const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));
const rows = () =>
  within(screen.getByRole("tree", { name: "Files" }))
    .getAllByRole("treeitem")
    .map((r) => r.getAttribute("data-path"));
// answer clicks a button of the confirmation that shows.
const answer = async (user: ReturnType<typeof userEvent.setup>, label: string) => {
  const dialog = await screen.findByRole("alertdialog");
  await user.click(within(dialog).getByRole("button", { name: label }));
};
const pane = (name: string) => screen.getByRole("tab", { name });
const editor = () => {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement) : null;
};

describe("the Files pane", () => {
  it("draws the vault as a tree whose folders open, and opens files", async () => {
    const { user } = await start();
    expect(rows()).toEqual(["Deep", "Notes.md", "Orchard.md", "Quince.md"]);
    fileRow("Deep/Seeds.md");
    expect(rows()).toContain("Deep/Seeds.md");
    expect(useWorkspace.getState().ws.files.expanded).toEqual(["Deep"]);
    await user.click(fileRow("Quince.md"));
    expect(tabs()).toEqual(["Quince"]);
    expect(fileRow("Quince.md").getAttribute("aria-selected")).toBe("true");
  });

  it("sorts by the menu's choice", async () => {
    const { user } = await start();
    await user.click(screen.getByRole("button", { name: "Change sort order" }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "File name (Z to A)" }));
    expect(rows()).toEqual(["Deep", "Quince.md", "Orchard.md", "Notes.md"]);
  });

  it("renames a file in place from its menu, keeping its extension", async () => {
    const { mock, user } = await start();
    fireEvent.contextMenu(fileRow("Deep/Seeds.md"));
    await user.click(screen.getByRole("menuitem", { name: "Rename…" }));
    const field = screen.getByRole("textbox", { name: "Name" });
    await user.clear(field);
    await user.type(field, "Beans{Enter}");
    expect(screen.getByRole("alertdialog", { name: "Update links?" })).toBeTruthy();
    await answer(user, "Don't update");
    await waitFor(() => expect(mock.vault(3).text("Deep/Beans.md")).toBe("Beans.\n"));
    expect(mock.vault(3).text("Deep/Seeds.md")).toBeUndefined();
  });

  it("makes a folder and names it", async () => {
    const { user } = await start();
    await user.click(screen.getByRole("button", { name: "New folder" }));
    const field = await screen.findByRole("textbox", { name: "Name" });
    expect((field as HTMLInputElement).value).toBe("Untitled");
    await user.clear(field);
    await user.type(field, "Archive{Enter}");
    await answer(user, "Don't update");
    await waitFor(() => expect(useFiles.getState().folders).toContain("Archive"));
    expect(useFiles.getState().folders).not.toContain("Untitled");
  });

  it("moves a file dropped on a folder", async () => {
    const { mock, user } = await start();
    const data = new Map([[DRAG_TYPE, JSON.stringify(["Notes.md"])]]);
    const dataTransfer = { types: [DRAG_TYPE], getData: (t: string) => data.get(t) ?? "", dropEffect: "" };
    fireEvent.dragOver(fileRow("Deep"), { dataTransfer });
    fireEvent.drop(fileRow("Deep"), { dataTransfer });
    await answer(user, "Update");
    await waitFor(() => expect(mock.vault(3).text("Deep/Notes.md")).toBe("Ask about quince prices.\n"));
  });

  it("deletes a file from its menu once the user agrees", async () => {
    const { mock, user } = await start();
    fireEvent.contextMenu(fileRow("Notes.md"));
    await user.click(screen.getByRole("menuitem", { name: "Delete" }));
    expect(screen.getByRole("alertdialog", { name: 'Delete "Notes"?' })).toBeTruthy();
    await answer(user, "Delete");
    await waitFor(() => expect(mock.vault(3).text("Notes.md")).toBeUndefined());
    await waitFor(() => expect(rows()).not.toContain("Notes.md"));
  });
});

describe("the Search pane", () => {
  it("searches as typed and opens a hit", async () => {
    const { user } = await start();
    await user.click(pane("Search"));
    await user.type(screen.getByRole("searchbox", { name: "Search" }), "quince");
    const results = await screen.findByRole("list", { name: "Results" }, { timeout: 3000 });
    expect(screen.getByText("3 results")).toBeTruthy();
    await user.click(within(results).getByRole("button", { name: "Notes" }));
    expect(tabs()).toEqual(["Notes"]);
  });

  it("opens with Ctrl+Shift+F, focused", async () => {
    const { user } = await start();
    await user.keyboard("{Control>}{Shift>}f{/Shift}{/Control}");
    expect(pane("Search").getAttribute("aria-selected")).toBe("true");
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" })));
  });
});

describe("the Tags and All properties panes", () => {
  it("search a clicked tag, nested tags included", async () => {
    const { user } = await start();
    await user.click(pane("Tags"));
    await user.click(await screen.findByRole("button", { name: "garden 2" }));
    expect(pane("Search").getAttribute("aria-selected")).toBe("true");
    expect((screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement).value).toBe("tag:#garden");
    const results = await screen.findByRole("list", { name: "Results" }, { timeout: 3000 });
    expect(within(results).getAllByRole("button", { name: /^(Orchard|Quince)$/ })).toHaveLength(2);
  });

  it("list the notes with a clicked property", async () => {
    const { user } = await start();
    await user.click(pane("All properties"));
    await user.click(await screen.findByRole("button", { name: /^status/ }));
    expect((screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement).value).toBe("[status]");
    const results = await screen.findByRole("list", { name: "Results" }, { timeout: 3000 });
    expect(within(results).getByRole("button", { name: "Orchard" })).toBeTruthy();
    expect(screen.getByText("1 result")).toBeTruthy();
  });
});

describe("the link panes", () => {
  it("show a note's backlinks with their lines, and its unlinked mentions", async () => {
    const { user } = await start();
    await user.click(fileRow("Quince.md"));
    const linked = await screen.findByRole("region", { name: "Linked mentions" });
    const line = await within(linked).findByRole("button", { name: "Plant the [[Quince]] soon, and a [[Pear]]." });
    expect([...line.querySelectorAll("mark")].map((m) => m.textContent)).toEqual(["[[Quince]]"]);
    const unlinked = screen.getByRole("region", { name: "Unlinked mentions" });
    await waitFor(() => expect(within(unlinked).getAllByRole("button")[0]?.textContent).toBe("Unlinked mentions1"));
    await user.click(within(unlinked).getAllByRole("button")[0] as HTMLElement);
    expect(within(unlinked).getByRole("button", { name: "Notes" })).toBeTruthy();
    await user.click(line);
    expect(tabs()).toEqual(["Orchard"]);
  });

  it("list a note's links, and create an unresolved one", async () => {
    const { mock, user } = await start();
    await user.click(fileRow("Orchard.md"));
    await user.click(pane("Outgoing links"));
    const list = await screen.findByRole("list", { name: "Outgoing links" });
    await waitFor(() =>
      expect(
        within(list)
          .getAllByRole("button")
          .map((b) => b.textContent),
      ).toEqual(["Quince", "Pear"]),
    );
    await user.click(within(list).getByRole("button", { name: "Pear" }));
    await waitFor(() => expect(mock.vault(3).text("Pear.md")).toBe(""));
    expect(tabs()).toEqual(["Pear"]);
  });
});

describe("the Outline pane", () => {
  it("lists the editor's headings and takes the cursor to one", async () => {
    const { user } = await start();
    await user.click(fileRow("Quince.md"));
    await waitFor(() => expect(editor()).not.toBeNull());
    await user.click(pane("Outline"));
    const tree = await screen.findByRole("tree", { name: "Outline" });
    await waitFor(() =>
      expect(
        within(tree)
          .getAllByRole("treeitem")
          .map((i) => i.textContent),
      ).toEqual(["Quince", "Pruning"]),
    );
    await user.click(within(tree).getByRole("button", { name: "Pruning" }));
    expect(editor()?.state.selection.main.head).toBe("# Quince\n\n".length);
    act(() => editor()?.dispatch({ changes: { from: 0, insert: "## Before\n" }, userEvent: "input.type" }));
    await waitFor(() =>
      expect(
        within(tree)
          .getAllByRole("treeitem")
          .map((i) => i.textContent),
      ).toEqual(["Before", "Quince", "Pruning"]),
    );
  });
});
