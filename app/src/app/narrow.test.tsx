import { EditorView } from "@codemirror/view";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { emptyWorkspace } from "../workspace/model";
import { activeTab, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";
import { NARROW } from "./narrow";

// A window as narrow as a phone: the media query matches.
function narrowWindow(matches: boolean) {
  vi.stubGlobal("matchMedia", (q: string) => ({
    matches: q === NARROW ? matches : false,
    media: q,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

beforeEach(() => narrowWindow(true));

afterEach(() => {
  vi.unstubAllGlobals();
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  usePrefs.getState().setPrefs({ lastVault: null });
  useOverlays.setState({ prompt: null, menu: null });
  localStorage.clear();
});

const files = { 3: { "a.md": "Alpha\n", "Deep/b.md": "Beta\n" }, 1: { "n.md": "N\n" } };

async function start() {
  render(<App connect={async () => new MockTransport(mockVaults(), { files })} />);
  await screen.findByRole("main", { name: "Workspace" });
}

const openNote = (w: Parameters<typeof open>[0], path: string) => open(w, { type: "note", path, mode: "edit" }, "here");

const editorText = () => {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement)?.state.doc.toString() : undefined;
};

const activePath = () => {
  const v = activeTab(useWorkspace.getState().ws).view;
  return v.type === "empty" ? "" : v.path;
};

describe("a narrow window", () => {
  it("takes the phone layout: a view header, the bottom bar, and no tab strip, ribbon or status bar", async () => {
    await start();
    expect(screen.getByRole("navigation", { name: "Bottom bar" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open left sidebar" })).toBeTruthy();
    expect(screen.queryByRole("tablist", { name: "Tabs" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Toggle left sidebar" })).toBeNull();
    expect(screen.queryByRole("complementary", { name: "Left sidebar" })).toBeNull();
  });

  it("opens a sidebar as a drawer, which closes when it opens a note", async () => {
    const user = userEvent.setup();
    await start();
    await user.click(screen.getByRole("button", { name: "Open left sidebar" }));
    const drawer = screen.getByRole("dialog", { name: "Left sidebar" });
    await user.click(await within(drawer).findByText("a"));
    await waitFor(() => expect(activePath()).toBe("a.md"));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Left sidebar" })).toBeNull());
    await user.click(screen.getByRole("button", { name: "Open right sidebar" }));
    expect(screen.getByRole("dialog", { name: "Right sidebar" })).toBeTruthy();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Right sidebar" })).toBeNull();
  });

  it("runs the bottom bar's buttons", async () => {
    const user = userEvent.setup();
    await start();
    await user.click(screen.getByRole("button", { name: "Open quick switcher" }));
    expect(useOverlays.getState().prompt).toBe("switcher");
    act(() => useOverlays.setState({ prompt: null }));
    const count = () =>
      Number(/\((\d+)\)/.exec(screen.getByRole("button", { name: /^Tabs / }).getAttribute("aria-label") ?? "")?.[1]);
    const before = count();
    await user.click(screen.getByRole("button", { name: "New tab" }));
    expect(count()).toBe(before + 1);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect(useOverlays.getState().menu?.label).toBe("Menu");
  });

  it("shows the editing toolbar in the bottom bar's place while an editor has the focus", async () => {
    const user = userEvent.setup();
    await start();
    act(() => useWorkspace.getState().update((w) => openNote(w, "a.md")));
    await waitFor(() => expect(editorText()).toBe("Alpha\n"));
    const view = EditorView.findFromDOM(screen.getByTestId("note-editor").querySelector(".cm-editor") as HTMLElement);
    act(() => view?.focus());
    const toolbar = await screen.findByRole("toolbar", { name: "Editing toolbar" });
    expect(screen.queryByRole("navigation", { name: "Bottom bar" })).toBeNull();
    const names = within(toolbar)
      .getAllByRole("button")
      .map((b) => b.getAttribute("aria-label"));
    expect(names.slice(0, 7)).toEqual([
      "Undo",
      "Redo",
      "Add internal link",
      "Add embed",
      "Add tag",
      "Attach file",
      "Set heading",
    ]);
    expect(names).toHaveLength(20);
    await user.click(within(toolbar).getByRole("button", { name: "Bold" }));
    await waitFor(() => expect(editorText()).toBe("**Alpha**\n"));
  });

  it("lists the tabs full screen from the bottom bar, to activate, close or add one", async () => {
    const user = userEvent.setup();
    await start();
    // In reading view, so no editor takes the focus (and the bottom bar).
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    await user.click(screen.getByRole("button", { name: "New tab" }));
    await user.click(screen.getByRole("button", { name: /^Tabs / }));
    const sw = screen.getByRole("dialog", { name: "Tabs" });
    const rows = within(sw).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    await user.click(within(sw).getByRole("button", { name: "a" }));
    expect(screen.queryByRole("dialog", { name: "Tabs" })).toBeNull();
    expect(activePath()).toBe("a.md");
    await user.click(screen.getByRole("button", { name: /^Tabs / }));
    await user.click(within(screen.getByRole("dialog", { name: "Tabs" })).getByRole("button", { name: "Close a" }));
    expect(within(screen.getByRole("dialog", { name: "Tabs" })).getAllByRole("listitem")).toHaveLength(1);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Tabs" })).toBeNull();
  });
});
