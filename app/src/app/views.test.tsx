// @vitest-environment jsdom
// The reading view, the merge view and the history in the window, over the
// mock: in jsdom, whose DOM DOMPurify reads as a browser's.
import { EditorView } from "@codemirror/view";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { DocumentRegistry } from "../editor/registry";
import { finish, toggleLine } from "../reading/ReadingView";
import { Client, PROTOCOL } from "../rpc/gen/api";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

// jsdom has no layout: CodeMirror measures text through Range's rectangles.
const noRects = () => Object.assign([], { item: () => null }) as unknown as DOMRectList;
Range.prototype.getClientRects ??= noRects;
Range.prototype.getBoundingClientRect ??= () => new DOMRect();

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  usePrefs.getState().setPrefs({ lastVault: null, loadRemoteImages: true });
  localStorage.clear();
});

const NOTE = "# Plan\n\nSee [[b]] and more.\n";

async function start() {
  const mock = new MockTransport(mockVaults(), { files: { 3: { "a.md": NOTE, "b.md": "Beta\n" } } });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  return mock;
}

const editor = () => {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement) : null;
};
const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));

describe("the reading view", () => {
  it("renders the document's text, saved or not, and follows its links", async () => {
    await start();
    const user = userEvent.setup();
    await user.click(await findFileRow("a.md"));
    await waitFor(() => expect(editor()).not.toBeNull());
    act(() =>
      editor()?.dispatch({ changes: { from: NOTE.length, insert: "Typed, not saved.\n" }, userEvent: "input.type" }),
    );
    await user.click(screen.getByRole("button", { name: "Reading view" }));
    const view = await screen.findByTestId("reading-view");
    await waitFor(() => expect(view.querySelector("h1")?.textContent).toBe("Plan"));
    expect(view.textContent).toContain("Typed, not saved.");
    const link = view.querySelector("a.internal-link");
    expect(link?.getAttribute("data-path")).toBe("b.md");
    fireEvent.click(link as Element);
    expect(tabs()).toEqual(["b"]);
  });

  it("names files by their blob URLs, draws math, drops remote images unless they load, and enables tasks", () => {
    const el = document.createElement("div");
    el.innerHTML =
      '<img class="embed" data-blob="ab12.png" alt="pic"><img src="https://e.test/r.png" alt="remote"><span class="math" data-tex="x^2"></span>' +
      '<ul><li><input type="checkbox" disabled="" data-line="3"> task</li></ul><div class="embed"><input type="checkbox" disabled=""></div>';
    finish(el, (hash, ext) => `blob://${hash}.${ext}`, false);
    expect(el.querySelector("img.embed")?.getAttribute("src")).toBe("blob://ab12.png");
    expect(el.querySelector('img[alt="remote"]')?.hasAttribute("src")).toBe(false);
    expect(el.querySelector(".math .katex")).not.toBeNull();
    // A disabled checkbox takes no clicks in a browser; an embedded note's has no line to toggle.
    expect([...el.querySelectorAll("input")].map((i) => i.disabled)).toEqual([false, true]);
  });
});

describe("a checkbox in the reading view", () => {
  it("toggles its task's line in the document, which saves", async () => {
    const mock = new MockTransport(mockVaults(), { files: { 3: { "t.md": "# Tasks\n\n- [ ] prune\n- [x] pick\n" } } });
    const client = new Client(mock);
    await client.rpc.hello({ protocol: PROTOCOL, client: "test" });
    const registry = new DocumentRegistry(() => client);
    const doc = registry.acquire(3, "t.md");
    await waitFor(() => expect(doc.isLoaded).toBe(true));
    toggleLine(doc, 3);
    toggleLine(doc, 4);
    toggleLine(doc, 1); // not a task: nothing
    expect(doc.text.toString()).toBe("# Tasks\n\n- [x] prune\n- [ ] pick\n");
    await waitFor(() => expect(mock.vault(3).text("t.md")).toBe("# Tasks\n\n- [x] prune\n- [ ] pick\n"), {
      timeout: 3000,
    });
  });
});

describe("the merge view", () => {
  it("opens from a conflict's Compare, against the version notesd kept, and Done closes it", async () => {
    const mock = await start();
    const user = userEvent.setup();
    await user.click(await findFileRow("a.md"));
    await waitFor(() => expect(editor()).not.toBeNull());
    act(() => editor()?.dispatch({ changes: { from: 2, insert: "My " }, userEvent: "input.type" }));
    act(() => mock.vault(3).outside("a.md", "# Their Plan\n\nSee [[b]] and more.\n"));
    const notice = await screen.findByText(/1 change conflicted with outside/, undefined, { timeout: 3000 });
    expect(editor()?.state.doc.toString()).toBe("# Their Plan\n\nSee [[b]] and more.\n");
    fireEvent.click(within(notice.parentElement as HTMLElement).getByRole("button", { name: "Compare" }));
    await waitFor(() => expect(tabs()).toEqual(["a", "Merge a"]));
    expect(await screen.findByText(/Comparing with the conflict version of/)).toBeTruthy();
    const merge = screen.getByTestId("merge-view");
    await waitFor(() => expect(merge.querySelectorAll(".cm-editor")).toHaveLength(2));
    const [left, right] = [...merge.querySelectorAll(".cm-editor")].map((e) =>
      EditorView.findFromDOM(e as HTMLElement),
    );
    expect(left?.state.doc.toString()).toBe("# Their Plan\n\nSee [[b]] and more.\n");
    expect(right?.state.doc.toString()).toBe("# My Plan\n\nSee [[b]] and more.\n");
    // Taking the right side's text is an edit of the note, which saves.
    act(() => left?.dispatch({ changes: { from: 2, to: 7, insert: "My" }, userEvent: "revert" }));
    await waitFor(() => expect(mock.vault(3).text("a.md")).toBe("# My Plan\n\nSee [[b]] and more.\n"), {
      timeout: 3000,
    });
    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(tabs()).toEqual(["a"]);
  });
});

describe("the history", () => {
  it("lists the versions, shows one, and restores it with the document's base", async () => {
    const mock = await start();
    act(() => mock.vault(3).outside("a.md", "# Plan, rewritten\n"));
    await act(async () => useWorkspace.getState().update((w) => open(w, { type: "history", path: "a.md" }, "tab")));
    const versions = await screen.findByRole("navigation", { name: "Versions" });
    await waitFor(() => expect(within(versions).getAllByRole("button")).toHaveLength(2));
    fireEvent.click(within(versions).getAllByRole("button")[1] as HTMLElement); // the older
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Version" }).textContent).toContain("See [[b]] and more."),
    );
    fireEvent.click(screen.getByRole("button", { name: "Restore this version" }));
    await waitFor(() => expect(mock.vault(3).text("a.md")).toBe(NOTE));
  });
});
