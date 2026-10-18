// @vitest-environment jsdom
// Live preview's widgets in the window, over the mock (batch 3's second
// part): every kind drawn in a note, rendered blocks filled by notesd's
// rendering and filled again when an embedded note changes, and a tag's
// click searching for it. In jsdom, whose DOM DOMPurify reads as a
// browser's: under happy-dom the sanitizer escapes everything.
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePanes } from "../data/panes";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { fileRow, findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
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
  useOverlays.setState({ menu: null, prompt: null, notices: [], recentCommands: [] });
  usePanes.setState({ searchQuery: "", searchFocus: 0 });
  usePrefs.getState().setPrefs({ lastVault: null });
  localStorage.clear();
});

const GARDEN = [
  "# Garden",
  "",
  "A ==bright== idea with #fruit and `code`.[^1]",
  "",
  "> [!tip] Water",
  "> In the morning.",
  "",
  "| a | b |",
  "| - | - |",
  "| 1 | 2 |",
  "",
  "![[Plan]]",
  "",
  "- item",
  "- [x] done",
  "",
  "---",
  "",
  "[^1]: A note.",
  "",
].join("\n");

const FILES = { 3: { "Garden.md": GARDEN, "Plan.md": "Rows of quince.\n" } };

async function open() {
  const mock = new MockTransport(mockVaults(), { files: FILES });
  render(<App connect={async () => mock} />);
  await screen.findByRole("main", { name: "Workspace" });
  await findFileRow("Garden.md");
  const user = userEvent.setup();
  await user.click(fileRow("Garden.md"));
  const editor = await screen.findByTestId("note-editor");
  await waitFor(() => expect(editor.querySelector(".cm-content")).not.toBeNull());
  return { mock, user, editor };
}

const texts = (root: Element, sel: string) => [...root.querySelectorAll(sel)].map((el) => el.textContent?.trim());

describe("live preview", () => {
  it("draws every kind of widget in a note", async () => {
    const { editor } = await open();
    expect(texts(editor, ".cm-pv-tag")).toEqual(["#fruit"]);
    expect(texts(editor, ".cm-pv-inline-code")).toEqual(["code"]);
    expect(texts(editor, "sup.cm-pv-footnote")).toEqual(["1"]);
    expect(editor.querySelectorAll(".cm-pv-bullet")).toHaveLength(1);
    expect(texts(editor, ".cm-pv-task-done")).toEqual(["done"]);
    expect(editor.querySelectorAll(".cm-pv-hr")).toHaveLength(1);
    await waitFor(() => expect(texts(editor, ".cm-pv-callout .callout-title")).toEqual(["Water"]));
    await waitFor(() => expect(editor.querySelectorAll(".cm-pv-table table td")).toHaveLength(2));
    await waitFor(() => expect(texts(editor, ".cm-pv-embed .embed p")).toEqual(["Rows of quince."]));
  });

  it("renders an embedded note again when it changes", async () => {
    const { mock, editor } = await open();
    await waitFor(() => expect(texts(editor, ".cm-pv-embed .embed p")).toEqual(["Rows of quince."]));
    act(() => mock.vault(3).outside("Plan.md", "Rows of pears.\n"));
    await waitFor(() => expect(texts(editor, ".cm-pv-embed .embed p")).toEqual(["Rows of pears."]));
  });

  it("searches for a tag when it is clicked", async () => {
    const { editor } = await open();
    const tag = editor.querySelector(".cm-pv-tag");
    act(() => void tag?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 })));
    await waitFor(() => expect(usePanes.getState().searchQuery).toBe("tag:#fruit"));
  });
});
