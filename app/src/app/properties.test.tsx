// Batch 3's last part in the window, over the mock: the properties block
// in live preview and its edits of the frontmatter's text, Add file
// property, the Properties in document preference, the inline title,
// Insert template and today's daily note.
import { EditorView } from "@codemirror/view";
import { act, render, screen, waitFor, within } from "@testing-library/react";
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
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, confirm: null, templates: null, notices: [], recentCommands: [] });
  usePrefs.getState().setPrefs({ ...DEFAULT_PREFS, lastVault: null });
  localStorage.clear();
});

const PLAN = "---\nstatus: draft\ntags:\n  - a\n---\nBody.\n";
const FILES = {
  3: {
    "Plan.md": PLAN,
    "Notes.md": "Ask.\n",
    "Templates/Meeting.md": "---\ntype: meeting\nstatus: x\n---\n## {{title}} notes\n",
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

const editor = () => screen.getByTestId("note-editor");
const block = () => within(editor()).getByRole("region", { name: "Properties" });
const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));

describe("the properties block", () => {
  it("draws the frontmatter as rows, the cursor starting after it", async () => {
    const { view: v } = await open("Plan.md");
    await waitFor(() =>
      expect(
        within(block())
          .getAllByRole("group")
          .map((g) => g.getAttribute("aria-label")),
      ).toEqual(["status", "tags"]),
    );
    expect(v.state.selection.main.head).toBe(PLAN.indexOf("Body"));
  });

  it("writes a value's edit and a removal into the frontmatter's text", async () => {
    const { user, view: v } = await open("Plan.md");
    const status = await within(editor()).findByRole("textbox", { name: "status" });
    await user.clear(status);
    await user.type(status, "done{Enter}");
    await waitFor(() => expect(v.state.doc.toString()).toBe("---\nstatus: done\ntags:\n  - a\n---\nBody.\n"));
    await user.click(within(block()).getByRole("button", { name: "Remove tags" }));
    await waitFor(() => expect(v.state.doc.toString()).toBe("---\nstatus: done\n---\nBody.\n"));
  });

  it("adds a property with Ctrl+;, making frontmatter where there is none", async () => {
    const { user, view: v } = await open("Notes.md");
    act(() => v.focus());
    await user.keyboard("{Control>};{/Control}");
    await waitFor(() => expect(v.state.doc.toString()).toBe("---\n---\nAsk.\n"));
    const name = await within(editor()).findByRole("textbox", { name: "New property name" });
    await user.type(name, "owner{Enter}");
    await waitFor(() => expect(v.state.doc.toString()).toBe('---\nowner: ""\n---\nAsk.\n'));
  });

  it("leaves the YAML, or hides it, as the preference says", async () => {
    const { view: v } = await open("Plan.md");
    await within(editor()).findByRole("region", { name: "Properties" });
    act(() => usePrefs.getState().setPrefs({ properties: "source" }));
    await waitFor(() => expect(within(editor()).queryByRole("region", { name: "Properties" })).toBeNull());
    expect(v.contentDOM.textContent).toContain("status: draft");
    act(() => usePrefs.getState().setPrefs({ properties: "hidden" }));
    await waitFor(() => expect(v.contentDOM.textContent).not.toContain("status: draft"));
    expect(within(editor()).queryByRole("region", { name: "Properties" })).toBeNull();
  });
});

describe("the inline title", () => {
  it("renames the note", async () => {
    const { mock, user } = await open("Notes.md");
    const title = within(editor()).getByRole("textbox", { name: "Title" });
    expect((title as HTMLInputElement).value).toBe("Notes");
    await user.clear(title);
    await user.type(title, "Questions{Enter}");
    const dialog = await screen.findByRole("alertdialog", { name: "Update links?" });
    await user.click(within(dialog).getByRole("button", { name: "Don't update" }));
    await waitFor(() => expect(mock.vault(3).text("Questions.md")).toBe("Ask.\n"));
    await waitFor(() => expect(tabs()).toEqual(["Questions"]));
  });

  it("goes away with its preference", async () => {
    await open("Notes.md");
    act(() => usePrefs.getState().setPrefs({ inlineTitle: false }));
    await waitFor(() => expect(within(editor()).queryByRole("textbox", { name: "Title" })).toBeNull());
  });
});

describe("templates and daily notes", () => {
  it("insert a template at the cursor, its properties the note lacks joining the note's", async () => {
    const { user, view: v } = await open("Plan.md");
    act(() => {
      const s = useVaultState.getState().settings;
      if (s) useVaultState.setState({ settings: { ...s, templateFolder: "Templates" } });
    });
    await user.click(screen.getByRole("button", { name: "Insert template" }));
    const chooser = await screen.findByRole("dialog", { name: "Insert template" });
    expect(
      within(chooser)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Meeting"]);
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(v.state.doc.toString()).toBe(
        "---\nstatus: draft\ntags:\n  - a\ntype: meeting\n---\n## Plan notes\nBody.\n",
      ),
    );
  });

  it("say where to choose a template folder when there is none", async () => {
    const { user } = await open("Plan.md");
    await user.click(screen.getByRole("button", { name: "Insert template" }));
    expect(await screen.findByText(/Choose a template folder in Settings/)).toBeTruthy();
  });

  it("open today's daily note, making it", async () => {
    const { mock, user } = await open("Notes.md");
    await user.click(screen.getByRole("button", { name: "Open today's daily note" }));
    await waitFor(() => expect(tabs()[0]).toMatch(/^\d{4}-\d{2}-\d{2}$/));
    expect(mock.vault(3).text(`${tabs()[0]}.md`)).toBe("");
  });
});
