import { EditorView } from "@codemirror/view";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { Client, PROTOCOL } from "../rpc/gen/api";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { fileRow, findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { useWorkspace, workspaceKey } from "../workspace/store";
import { App } from "./App";
import { fitSidebars } from "./MainWindow";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  usePrefs.getState().setPrefs({ lastVault: null });
  localStorage.clear();
});

const files = { 3: { "a.md": "Alpha links [[b]].\n", "Deep/b.md": "Beta\n", "pic.png": "png" }, 1: { "n.md": "N\n" } };

async function start(mock = new MockTransport(mockVaults(), { files })) {
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  return mock;
}

const tabs = (group = 0) =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[group] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));
/** editorText is the active editor's document. */
const editorText = () => {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement)?.state.doc.toString() : undefined;
};

describe("the main window over the mock", () => {
  it("shows the window's vault, its files, and an empty tab", async () => {
    await start(new MockTransport(mockVaults(), { files, server: "notesd test" }));
    expect(screen.getByRole("button", { name: /garden/ })).toBeTruthy(); // the vault row: "garden" sorts first
    await waitFor(() => expect(fileRow("Deep/b.md")).toBeTruthy());
    expect(tabs()).toEqual(["New tab"]);
    expect(screen.getByRole("button", { name: "Create new note (Ctrl + N)" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe(""); // ready: nothing to say
    expect(screen.getByRole("button", { name: "Open quick switcher" }).hasAttribute("disabled")).toBe(false);
    expect(screen.getByRole("button", { name: "Open today's daily note" }).hasAttribute("disabled")).toBe(false);
    // Insert template needs a note open in an editor.
    expect(screen.getByRole("button", { name: "Insert template" }).hasAttribute("disabled")).toBe(true);
  });

  it("opens files from the Files pane as links open, a note in its editor", async () => {
    await start();
    await userEvent.setup().click(await findFileRow("a.md"));
    expect(tabs()).toEqual(["a"]);
    await waitFor(() => expect(editorText()).toBe("Alpha links [[b]].\n"));
    fireEvent.click(fileRow("Deep/b.md"), { ctrlKey: true });
    expect(tabs()).toEqual(["a", "b"]);
    expect(
      within(screen.getByRole("navigation", { name: "Breadcrumb" })).getByRole("button", { name: "Deep" }),
    ).toBeTruthy();
    fireEvent.click(fileRow("pic.png"), { ctrlKey: true, altKey: true });
    expect(screen.getAllByRole("tablist", { name: "Tabs" })).toHaveLength(2);
    expect(tabs(1)).toEqual(["pic.png"]);
    expect(screen.getByRole("img", { name: "pic.png" }).getAttribute("src")).toMatch(/^data:image\/png;base64,/);
  });

  it("runs the keymap's commands and the chrome's buttons", async () => {
    await start();
    const user = userEvent.setup();
    await user.click(await findFileRow("a.md"));
    await user.keyboard("{Control>}t{/Control}");
    expect(tabs()).toEqual(["a", "New tab"]);
    await user.keyboard("{Control>}w{/Control}");
    expect(tabs()).toEqual(["a"]);
    await user.keyboard("{Control>}{Shift>}t{/Shift}{/Control}");
    expect(tabs()).toEqual(["a", "New tab"]);
    await user.click(screen.getByRole("button", { name: "Toggle left sidebar" }));
    expect(screen.queryByRole("complementary", { name: "Left sidebar" })).toBeNull();
    await user.click(screen.getByRole("tab", { name: "Outline" }));
    expect(screen.getByRole("tab", { name: "Outline" }).getAttribute("aria-selected")).toBe("true");
  });

  it("creates a note with Ctrl+N and opens it", async () => {
    const mock = await start();
    await findFileRow("a.md");
    await userEvent.setup().keyboard("{Control>}n{/Control}");
    await waitFor(() => expect(tabs()).toEqual(["Untitled"]));
    expect(mock.vault(3).text("Untitled.md")).toBe("");
  });

  it("keeps a workspace per vault, and switches vaults from the vault row", async () => {
    await start();
    const user = userEvent.setup();
    await user.click(await findFileRow("a.md"));
    await waitFor(() => expect(localStorage.getItem(workspaceKey("/home/demo/garden"))).toContain('"a.md"'));
    await user.click(screen.getByRole("button", { name: /garden/ }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "Notes" }));
    await waitFor(() => expect(fileRow("n.md")).toBeTruthy());
    expect(tabs()).toEqual(["New tab"]);
    await user.click(screen.getByRole("button", { name: /Notes/ }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "garden" }));
    await waitFor(() => expect(tabs()).toEqual(["a"]));
  });

  it("follows a move into its tabs", async () => {
    const mock = await start();
    await userEvent.setup().click(await findFileRow("a.md"));
    const notesctl = new Client(mock);
    await notesctl.rpc.hello({ protocol: PROTOCOL, client: "notesctl" });
    await act(async () => {
      await notesctl.note.move({ vaultId: 3, path: "a.md", to: "Moved/Renamed.md" });
    });
    await waitFor(() => expect(tabs()).toEqual(["Renamed"]));
  });

  it("moves a dragged tab to another group", async () => {
    await start();
    await userEvent.setup().click(await findFileRow("a.md"));
    fireEvent.click(fileRow("Deep/b.md"), { ctrlKey: true });
    fireEvent.click(fileRow("pic.png"), { ctrlKey: true, altKey: true });
    const [left, right] = screen.getAllByRole("tablist", { name: "Tabs" }) as HTMLElement[];
    const dataTransfer = { setData: () => {}, effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(within(left as HTMLElement).getByRole("tab", { name: "b" }), { dataTransfer });
    fireEvent.dragOver(right as HTMLElement, { dataTransfer });
    fireEvent.drop(right as HTMLElement, { dataTransfer });
    expect(tabs(0)).toEqual(["a"]);
    expect(tabs(1)).toEqual(["pic.png", "b"]);
  });

  it("says how to add a vault when there is none", async () => {
    await start(new MockTransport());
    expect(screen.getByText(/No vaults yet/)).toBeTruthy();
  });

  it("shows the reason and reconnects when the connection is lost", async () => {
    const first = new MockTransport(mockVaults());
    let mock = first;
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    mock = new MockTransport([], { server: "notesd again" });
    act(() => first.close("notesd stopped"));
    // The window stays, so open editors keep what was typed; the status bar says why.
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toContain("Reconnecting to notesd… (notesd stopped)"),
    );
    expect(screen.getByRole("main", { name: "Workspace" })).toBeTruthy();
    await waitFor(() => expect(screen.getByText(/No vaults yet/)).toBeTruthy(), { timeout: 3000 });
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("saves what is typed in a note's editor", async () => {
    const mock = await start();
    await userEvent.setup().click(await findFileRow("a.md"));
    await waitFor(() => expect(editorText()).toBe("Alpha links [[b]].\n"));
    const view = EditorView.findFromDOM(screen.getByTestId("note-editor").querySelector(".cm-editor") as HTMLElement);
    act(() => view?.dispatch({ changes: { from: 0, insert: "Typed. " }, userEvent: "input.type" }));
    await waitFor(() => expect(mock.vault(3).text("a.md")).toBe("Typed. Alpha links [[b]].\n"), { timeout: 3000 });
  });
});

describe("fitSidebars", () => {
  const open = (width: number) => ({ open: true, width });
  it("gives way to the workspace, the right sidebar first, then collapses it, then the left", () => {
    expect(fitSidebars(1200, open(260), open(300))).toEqual({ left: 260, right: 300 });
    expect(fitSidebars(900, open(260), open(300))).toEqual({ left: 260, right: 276 }); // 900 - 44 - 320 = 536
    expect(fitSidebars(800, open(260), open(300))).toEqual({ left: 236, right: 200 });
    expect(fitSidebars(700, open(260), open(300))).toEqual({ left: 260, right: 0 });
    expect(fitSidebars(500, open(260), open(300))).toEqual({ left: 0, right: 0 });
    expect(fitSidebars(700, { open: false, width: 260 }, open(300))).toEqual({ left: 0, right: 300 });
  });
});
