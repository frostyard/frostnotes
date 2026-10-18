// The editor's batch 3 wiring, over the mock: Obsidian's formatting keys,
// following the link under the cursor, the status bar's counts, and link
// and tag completion.
import { acceptCompletion, currentCompletions, startCompletion } from "@codemirror/autocomplete";
import { EditorSelection } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

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
  usePrefs.getState().setPrefs({ lastVault: null });
  localStorage.clear();
});

const FILES = {
  3: {
    "Orchard.md": "# Orchard\n\nPlant the [[Quince]] soon. #garden\n",
    "Quince.md": "# Quince\n\n## Pruning\n\nCut in winter. ^cut\n",
    "Notes.md": "Ask about quince prices.\n",
  },
};

async function open(path: string) {
  const mock = new MockTransport(mockVaults(), { files: FILES });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  await findFileRow(path);
  const user = userEvent.setup();
  await user.click(fileRow(path));
  await waitFor(() => expect(view()?.state.doc.length).toBeGreaterThan(0));
  return { mock, user, view: view() as EditorView };
}

function view(): EditorView | null {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement) : null;
}

const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));

/** accept takes the selected completion, once CodeMirror allows it (75 ms after the list opens). */
async function accept(v: EditorView) {
  await waitFor(() => {
    let taken = false;
    act(() => {
      taken = acceptCompletion(v);
    });
    expect(taken).toBe(true);
  });
}

/** at puts the cursor where text is in the note, plus offset. */
function at(v: EditorView, text: string, offset = 0) {
  act(() => {
    v.focus();
    v.dispatch({ selection: { anchor: v.state.doc.toString().indexOf(text) + offset } });
  });
}

describe("formatting", () => {
  it("runs Obsidian's keys on the editor", async () => {
    const { user, view: v } = await open("Notes.md");
    at(v, "quince", 2);
    await user.keyboard("{Control>}b{/Control}");
    expect(v.state.doc.toString()).toBe("Ask about **quince** prices.\n");
    await user.keyboard("{Control>}l{/Control}");
    expect(v.state.doc.toString()).toBe("- [ ] Ask about **quince** prices.\n");
    await user.keyboard("{Control>}l{/Control}");
    expect(v.state.doc.toString()).toBe("- [x] Ask about **quince** prices.\n");
    await user.keyboard("{Control>}d{/Control}");
    expect(v.state.doc.toString()).toBe("");
  });
});

describe("the link under the cursor", () => {
  it("follows with Alt+Enter, and opens in a new tab with Ctrl+Enter", async () => {
    const { user, view: v } = await open("Orchard.md");
    at(v, "Quince]]");
    await user.keyboard("{Control>}{Enter}{/Control}");
    await waitFor(() => expect(tabs()).toEqual(["Orchard", "Quince"]));
    await user.click(screen.getByRole("tab", { name: "Orchard" }));
    const w = view() as EditorView;
    at(w, "Quince]]");
    await user.keyboard("{Alt>}{Enter}{/Alt}");
    await waitFor(() => expect(tabs()).toEqual(["Quince", "Quince"]));
  });
});

describe("the status bar's counts", () => {
  it("counts the note, then the selection", async () => {
    const { view: v } = await open("Notes.md");
    expect(await screen.findByText("4 words")).toBeTruthy();
    expect(screen.getByText("24 characters")).toBeTruthy();
    // A focused editor writes the DOM's selection, and happy-dom answers
    // that at once, inside the update: select with the editor blurred.
    act(() => {
      v.contentDOM.blur();
      v.dispatch({ selection: EditorSelection.range(10, 23) });
    });
    expect(await screen.findByText("2 words")).toBeTruthy();
    expect(screen.getByText("13 characters")).toBeTruthy();
  });
});

describe("completion", () => {
  it("completes a link to a note and its heading", async () => {
    const { view: v } = await open("Notes.md");
    act(() => {
      v.focus();
      v.dispatch({ changes: { from: 0, insert: "See [[qu" }, selection: { anchor: 8 }, userEvent: "input.type" });
      startCompletion(v);
    });
    await waitFor(() => expect(currentCompletions(v.state).map((c) => c.label)).toEqual(["Quince"]));
    await accept(v);
    expect(v.state.doc.toString()).toBe("See [[Quince]]Ask about quince prices.\n");
    act(() => {
      v.dispatch({ changes: { from: 12, insert: "#pr" }, selection: { anchor: 15 }, userEvent: "input.type" });
      startCompletion(v);
    });
    await waitFor(() => expect(currentCompletions(v.state).map((c) => c.label)).toEqual(["Pruning"]));
    await accept(v);
    expect(v.state.doc.toString()).toBe("See [[Quince#Pruning]]Ask about quince prices.\n");
  });

  it("completes a block ID and a tag", async () => {
    const { view: v } = await open("Notes.md");
    act(() => {
      v.focus();
      v.dispatch({ changes: { from: 0, insert: "[[Quince#^" }, selection: { anchor: 10 }, userEvent: "input.type" });
      startCompletion(v);
    });
    await waitFor(() => expect(currentCompletions(v.state).map((c) => c.label)).toEqual(["cut"]));
    await accept(v);
    expect(v.state.doc.toString()).toBe("[[Quince#^cut]]Ask about quince prices.\n");
    act(() => {
      v.dispatch({ changes: { from: 15, insert: " #ga" }, selection: { anchor: 19 }, userEvent: "input.type" });
      startCompletion(v);
    });
    await waitFor(() => expect(currentCompletions(v.state).map((c) => c.label)).toEqual(["garden"]));
  });
});
