// @vitest-environment jsdom
// What Phase 6 finished, in the window over the mock: zoom, web links and
// files opening outside the app in a browser, the browser's connection
// item, aliases in the quick switcher, page
// previews in the editor and the reading view, and the reading view's
// properties block. In jsdom, whose DOM DOMPurify reads as a browser's.
import { EditorView } from "@codemirror/view";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useAliases } from "../data/aliases";
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

// jsdom has no layout: CodeMirror measures text through Range's rectangles.
const noRects = () => Object.assign([], { item: () => null }) as unknown as DOMRectList;
Range.prototype.getClientRects ??= noRects;
Range.prototype.getBoundingClientRect ??= () => new DOMRect();

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, confirm: null, templates: null, preview: null, notices: [] });
  useAliases.setState({ vaultId: null, byPath: new Map() });
  usePrefs.getState().setPrefs({ ...DEFAULT_PREFS, lastVault: null });
  document.documentElement.style.zoom = "";
  localStorage.clear();
  vi.restoreAllMocks();
});

const FILES = {
  3: {
    "Plan.md": "---\nstatus: draft\naliases:\n  - Roadmap\n---\nRows of quince.\n",
    "Notes.md": "Intro.\nSee [[Plan]] and [site](https://example.test/).\n",
    "pic.png": "not really a png",
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
  return { mock, user };
}

function view(): EditorView | null {
  const el = screen.queryByTestId("note-editor")?.querySelector(".cm-editor");
  return el ? EditorView.findFromDOM(el as HTMLElement) : null;
}

const tabs = () =>
  within(screen.getAllByRole("tablist", { name: "Tabs" })[0] as HTMLElement)
    .getAllByRole("tab")
    .map((t) => t.getAttribute("aria-label"));

describe("zoom", () => {
  it("steps in and out with Ctrl+= and Ctrl+-, and resets with Ctrl+0", async () => {
    const { user } = await open("Notes.md");
    await user.keyboard("{Control>}={/Control}");
    expect(usePrefs.getState().zoom).toBe(1.1);
    expect(document.documentElement.style.zoom).toBe("1.1");
    await user.keyboard("{Control>}-{/Control}{Control>}-{/Control}");
    expect(usePrefs.getState().zoom).toBe(0.9);
    await user.keyboard("{Control>}0{/Control}");
    expect(usePrefs.getState().zoom).toBe(1);
    expect(document.documentElement.style.zoom).toBe("");
  });
});

describe("outside the app, in a browser", () => {
  it("opens a web link in a new tab, and a file the browser shows in a new browser tab", async () => {
    const opened = vi.spyOn(window, "open").mockReturnValue(null);
    const { user } = await open("Notes.md");
    const site = await waitFor(() => {
      const el = [...(view()?.contentDOM.querySelectorAll<HTMLElement>(".cm-pv-link") ?? [])].find(
        (l) => l.textContent === "site",
      );
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    fireEvent.mouseDown(site, { button: 0 });
    await waitFor(() => expect(opened).toHaveBeenCalledWith("https://example.test/", "_blank", "noopener,noreferrer"));
    // A note has no way out of the app in a browser (docs/specs/ui.md#in-a-browser).
    await user.click(screen.getByRole("button", { name: "More options" }));
    expect(screen.queryByRole("menuitem", { name: /^Open in (default app|new browser tab)$/ })).toBeNull();
    await user.keyboard("{Escape}");
    // An image opens by its content URL in a new browser tab.
    fireEvent.contextMenu(fileRow("pic.png"));
    expect(screen.queryByRole("menuitem", { name: "Open in default app" })).toBeNull();
    await user.click(await screen.findByRole("menuitem", { name: "Open in new browser tab" }));
    await waitFor(() =>
      expect(opened).toHaveBeenLastCalledWith(
        expect.stringMatching(/^data:image\/png;base64,/),
        "_blank",
        "noopener,noreferrer",
      ),
    );
  });

  it("shows notes' connection in the status bar, and who is signed in in its menu", async () => {
    const mock = new MockTransport(mockVaults(), {
      files: FILES,
      caller: "session",
      login: "ana@example.com",
      displayName: "Ana",
      node: "phone",
    });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Connected as ana@example.com on phone" }));
    const labels = useOverlays.getState().menu?.items.map((i) => (i.kind === "item" ? i.label : "-"));
    expect(labels).toEqual(["Signed in as Ana", "ana@example.com", "on phone"]);
  });
});

describe("aliases", () => {
  it("find a note in the quick switcher", async () => {
    const { user } = await open("Notes.md");
    await waitFor(() => expect(useAliases.getState().byPath.get("Plan.md")).toEqual(["Roadmap"]));
    await user.keyboard("{Control>}o{/Control}");
    await user.keyboard("roadm");
    const options = within(screen.getByRole("listbox")).getAllByRole("option");
    expect(options[0]?.textContent).toContain("Roadmap");
  });
});

describe("page previews", () => {
  it("show the note a link in the editor leads to while Ctrl is held, and open it", async () => {
    const { user } = await open("Notes.md");
    const link = await waitFor(() => {
      const el = view()?.contentDOM.querySelector<HTMLElement>('.cm-pv-link[data-target="Plan"]');
      expect(el?.classList.contains("cm-pv-unresolved")).toBe(false);
      return el as HTMLElement;
    });
    fireEvent.mouseMove(link);
    expect(screen.queryByRole("dialog", { name: "Preview of Plan" })).toBeNull();
    fireEvent.mouseMove(link, { ctrlKey: true });
    const preview = await screen.findByRole("dialog", { name: "Preview of Plan" });
    await waitFor(() => expect(preview.textContent).toContain("Rows of quince."));
    await user.click(within(preview).getByRole("button", { name: "Plan" }));
    await waitFor(() => expect(tabs()).toEqual(["Plan"]));
  });

  it("show the note a link in the reading view leads to", async () => {
    const { user } = await open("Notes.md");
    await user.click(screen.getByRole("button", { name: "Reading view" }));
    const reading = await screen.findByTestId("reading-view");
    const link = await waitFor(() => {
      const el = reading.querySelector<HTMLElement>('a.internal-link[data-path="Plan.md"]');
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    fireEvent.mouseMove(link, { ctrlKey: true });
    const preview = await screen.findByRole("dialog", { name: "Preview of Plan" });
    await waitFor(() => expect(preview.textContent).toContain("Rows of quince."));
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Preview of Plan" })).toBeNull();
  });
});

describe("the reading view's properties", () => {
  it("draw the frontmatter and write its edits into the note", async () => {
    const { mock, user } = await open("Plan.md");
    await user.click(screen.getByRole("button", { name: "Reading view" }));
    const block = await screen.findByRole("region", { name: "Properties" });
    expect(
      within(block)
        .getAllByRole("group")
        .map((g) => g.getAttribute("aria-label")),
    ).toEqual(["status", "aliases"]);
    const status = within(block).getByRole("textbox", { name: "status" });
    await user.clear(status);
    await user.type(status, "done{Enter}");
    await waitFor(() => expect(mock.vault(3).text("Plan.md")).toContain("status: done\n"));
    act(() => usePrefs.getState().setPrefs({ properties: "hidden" }));
    expect(screen.queryByRole("region", { name: "Properties" })).toBeNull();
  });
});
