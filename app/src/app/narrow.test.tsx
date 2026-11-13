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
import { NARROW_BACKLINKS } from "./menus";
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
  const mock = new MockTransport(mockVaults(), { files });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  return mock;
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
    // The prompt's field had the focus, and the keyboard with it: the bottom bar comes back.
    await screen.findByRole("navigation", { name: "Bottom bar" });
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

  it("hides the bottom bar while a text field outside the editor has the focus", async () => {
    await start();
    // In reading view, so no editor takes the focus.
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    const title = await screen.findByRole("textbox", { name: "Title" });
    expect(screen.getByRole("navigation", { name: "Bottom bar" })).toBeTruthy();
    act(() => title.focus());
    await waitFor(() => expect(screen.queryByRole("navigation", { name: "Bottom bar" })).toBeNull());
    // The field is not the editor: the editing toolbar stays away.
    expect(screen.queryByRole("toolbar", { name: "Editing toolbar" })).toBeNull();
    act(() => title.blur());
    await screen.findByRole("navigation", { name: "Bottom bar" });
  });

  it("ends a prompt at the on-screen keyboard", async () => {
    // A keyboard 344 pixels high: the visual viewport ends above the layout viewport's 844.
    const resized = new Set<() => void>();
    const viewport = {
      height: 500,
      offsetTop: 0,
      addEventListener: (type: string, fn: () => void) => type === "resize" && resized.add(fn),
      removeEventListener: (_: string, fn: () => void) => resized.delete(fn),
    };
    vi.stubGlobal("visualViewport", viewport);
    vi.stubGlobal("innerHeight", 844);
    await start();
    act(() => useOverlays.setState({ prompt: "switcher" }));
    const dialog = await screen.findByRole("dialog", { name: "Quick switcher" });
    // Under its top margin, the top safe area, it ends at the keyboard.
    expect(dialog.style.maxHeight).toBe("calc(100dvh - env(safe-area-inset-top, 0px) - 344px)");
    // The keyboard closes: the prompt takes lodge-ui's height again.
    viewport.height = 844;
    act(() => {
      for (const fn of resized) fn();
    });
    expect(dialog.style.maxHeight).toBe("");
  });

  it("opens the right drawer on the Backlinks pane from the Menu's backlinks", async () => {
    const user = userEvent.setup();
    await start();
    // In reading view, so no editor takes the focus (and the bottom bar).
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    await user.click(screen.getByRole("button", { name: "Menu" }));
    act(() => useOverlays.getState().menu?.onChoose(NARROW_BACKLINKS));
    expect(screen.getByRole("dialog", { name: "Right sidebar" })).toBeTruthy();
    const right = useWorkspace.getState().ws.right;
    expect(right.panes[right.active]).toBe("backlinks");
  });

  it("shows the note's folders on a tap of the view header's title, a folder revealing it in the left drawer", async () => {
    const user = userEvent.setup();
    await start();
    act(() =>
      useWorkspace.getState().update((w) => open(w, { type: "note", path: "Deep/b.md", mode: "read" }, "here")),
    );
    await user.click(await screen.findByRole("button", { name: "b: show its folders" }));
    const crumbs = screen.getByRole("navigation", { name: "Breadcrumb" });
    await user.click(within(crumbs).getByRole("button", { name: "Deep" }));
    expect(screen.getByRole("dialog", { name: "Left sidebar" })).toBeTruthy();
  });

  it("stacks the merge view's two sides, the document above, without arrows", async () => {
    const mock = await start();
    act(() => mock.vault(3).outside("a.md", "Alpha, again\n"));
    const older = mock.vault(3).versionList("a.md").at(-1);
    if (!older) throw new Error("a.md has no versions");
    act(() =>
      useWorkspace
        .getState()
        .update((w) => open(w, { type: "merge", path: "a.md", other: { kind: "version", version: older.id } }, "here")),
    );
    const merge = await screen.findByTestId("merge-view");
    await waitFor(() => expect(merge.querySelectorAll(".cm-editor")).toHaveLength(2));
    expect(merge.dataset.stacked).toBe("true");
    expect(merge.className).toContain("[&_.cm-mergeViewEditors]:flex-col");
    expect(merge.querySelector(".cm-merge-revert")).toBeNull();
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

describe("the split commands", () => {
  // A narrow window draws one group: a split there would not show.
  it.each([
    ["a narrow window", true],
    ["a wide window", false],
  ])("in %s", async (_, narrow) => {
    narrowWindow(narrow);
    const user = userEvent.setup();
    await start();
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    await user.click(await screen.findByRole("button", { name: "More options" }));
    const menu = (useOverlays.getState().menu?.items ?? []).flatMap((i) => (i.kind === "item" ? [i.label] : []));
    expect(menu).toContain("Copy path");
    expect(menu.includes("Split right")).toBe(!narrow);
    expect(menu.includes("Split down")).toBe(!narrow);
    act(() => useOverlays.setState({ menu: null, prompt: "palette" }));
    const palette = await screen.findByRole("dialog", { name: "Command palette" });
    const names = within(palette)
      .getAllByRole("option")
      .map((o) => o.textContent ?? "");
    expect(names.some((n) => n.startsWith("Toggle pin"))).toBe(true);
    expect(names.some((n) => n.startsWith("Split right"))).toBe(!narrow);
    expect(names.some((n) => n.startsWith("Split down"))).toBe(!narrow);
  });
});

describe("a menu opened in a wide window", () => {
  it("splits nothing once the window is narrow", async () => {
    narrowWindow(false);
    const user = userEvent.setup();
    await start();
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    await user.click(await screen.findByRole("button", { name: "More options" }));
    const menu = useOverlays.getState().menu;
    expect(menu?.items.some((i) => i.kind === "item" && i.id === "split-right")).toBe(true);
    // The window turns narrow (a rotation) with the menu still open.
    narrowWindow(true);
    const before = JSON.stringify(useWorkspace.getState().ws.root);
    act(() => {
      menu?.onChoose("split-right");
      menu?.onChoose("split-down");
    });
    expect(JSON.stringify(useWorkspace.getState().ws.root)).toBe(before);
  });

  it("closes when the window turns narrow", async () => {
    let matches = false;
    const changed = new Set<(event: { matches: boolean }) => void>();
    vi.stubGlobal("matchMedia", (q: string) => ({
      matches: q === NARROW ? matches : false,
      media: q,
      addEventListener: (_: string, fn: (event: { matches: boolean }) => void) => q === NARROW && changed.add(fn),
      removeEventListener: (_: string, fn: (event: { matches: boolean }) => void) => changed.delete(fn),
    }));
    const user = userEvent.setup();
    await start();
    act(() => useWorkspace.getState().update((w) => open(w, { type: "note", path: "a.md", mode: "read" }, "here")));
    await user.click(await screen.findByRole("button", { name: "More options" }));
    expect(useOverlays.getState().menu?.label).toBe("More options");
    matches = true;
    act(() => {
      for (const fn of changed) fn({ matches });
    });
    await screen.findByRole("navigation", { name: "Bottom bar" });
    expect(useOverlays.getState().menu).toBeNull();
  });
});
