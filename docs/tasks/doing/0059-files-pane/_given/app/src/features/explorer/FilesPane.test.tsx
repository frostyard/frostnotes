import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { type TreeRow, treeRows } from "../../lib/fileTree";
import { DRAG_TYPE, type FilesCommand, FilesPane, type FilesPaneProps } from "./FilesPane";

const FILES = [
  { path: "Inbox.md", modifiedAt: 1 },
  { path: "Projects/Orchard plan.md", modifiedAt: 2 },
  { path: "Projects/Seeds.png", modifiedAt: 3 },
  { path: "Projects/Archive/Old.md", modifiedAt: 4 },
];
const rows = (expanded: string[] = ["Projects"]) =>
  treeRows(FILES, ["Empty"], { sort: "alphabetical", expanded: new Set(expanded), showUnsupported: false });

function setup(over: Partial<FilesPaneProps> = {}) {
  const commands: FilesCommand[] = [];
  const props: FilesPaneProps = {
    rows: rows(),
    active: "Projects/Orchard plan.md",
    anyExpanded: true,
    indentGuides: true,
    renaming: null,
    loaded: true,
    onCommand: (c) => commands.push(c),
    ...over,
  };
  const view = render(<FilesPane {...props} />);
  return { commands, props, view, user: userEvent.setup() };
}

const items = () => within(screen.getByRole("tree", { name: "Files" })).getAllByRole("treeitem");
const item = (name: string) => within(screen.getByRole("tree", { name: "Files" })).getByRole("treeitem", { name });
const NONE = { ctrl: false, alt: false, middle: false };

function dataTransfer(data: Record<string, string> = {}) {
  const store = new Map(Object.entries(data));
  return {
    get types() {
      return [...store.keys()];
    },
    setData: (t: string, v: string) => void store.set(t, v),
    getData: (t: string) => store.get(t) ?? "",
    effectAllowed: "",
    dropEffect: "",
    store,
  };
}

describe("FilesPane", () => {
  it("draws the rows as a tree: folders with chevrons, files with badges for other types", () => {
    setup();
    expect(items().map((i) => i.getAttribute("aria-label"))).toEqual([
      "Empty",
      "Projects",
      "Archive",
      "Orchard plan",
      "Seeds",
      "Inbox",
    ]);
    expect(items().map((i) => i.getAttribute("aria-level"))).toEqual(["1", "1", "2", "2", "2", "1"]);
    expect(items().map((i) => i.getAttribute("aria-expanded"))).toEqual(["false", "true", "false", null, null, null]);
    expect(items().map((i) => i.style.paddingLeft)).toEqual(["4px", "4px", "20px", "20px", "20px", "4px"]);
    expect(item("Seeds").textContent).toBe("Seedspng");
    const badge = within(item("Seeds")).getByText("png");
    expect(badge.className).toContain("uppercase");
    expect(badge.className).toContain("bg-badge");
    expect(item("Orchard plan").textContent).toBe("Orchard plan");
    expect(item("Projects").querySelector("svg")?.getAttribute("class")).toContain("rotate-90");
    expect(item("Archive").querySelector("svg")?.getAttribute("class")).not.toContain("rotate-90");
    expect(item("Inbox").getAttribute("title")).toBe("Inbox.md");
  });

  it("marks the active file and lets only one row take the tab key", () => {
    setup();
    expect(item("Orchard plan").getAttribute("aria-selected")).toBe("true");
    expect(item("Orchard plan").className).toContain("bg-selection-sidebar");
    expect(item("Inbox").getAttribute("aria-selected")).toBe("false");
    expect(items().map((i) => i.tabIndex)).toEqual([-1, -1, -1, 0, -1, -1]);
  });

  it("draws indentation guides, one per level, only when asked", () => {
    const { view } = setup();
    expect(item("Archive").querySelectorAll("[data-guide]").length).toBe(1);
    expect(item("Inbox").querySelectorAll("[data-guide]").length).toBe(0);
    view.unmount();
    setup({ indentGuides: false });
    expect(item("Archive").querySelectorAll("[data-guide]").length).toBe(0);
  });

  it("opens a file by click, Ctrl+click, Ctrl+Alt+click and middle click, and toggles a folder", () => {
    const { commands } = setup();
    fireEvent.click(item("Inbox"));
    fireEvent.click(item("Inbox"), { ctrlKey: true });
    fireEvent.click(item("Inbox"), { ctrlKey: true, altKey: true });
    fireEvent(item("Inbox"), new MouseEvent("auxclick", { bubbles: true, button: 1 }));
    fireEvent.click(item("Archive"));
    expect(commands).toEqual([
      { kind: "open", path: "Inbox.md", click: NONE },
      { kind: "open", path: "Inbox.md", click: { ctrl: true, alt: false, middle: false } },
      { kind: "open", path: "Inbox.md", click: { ctrl: true, alt: true, middle: false } },
      { kind: "open", path: "Inbox.md", click: { ctrl: false, alt: false, middle: true } },
      { kind: "toggle", path: "Projects/Archive" },
    ]);
  });

  it("moves focus with the arrows, Home and End, keeping the keys from the window", () => {
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    try {
      setup();
      item("Orchard plan").focus();
      fireEvent.keyDown(item("Orchard plan"), { key: "ArrowDown" });
      expect(document.activeElement).toBe(item("Seeds"));
      fireEvent.keyDown(item("Seeds"), { key: "ArrowUp" });
      fireEvent.keyDown(item("Orchard plan"), { key: "ArrowUp" });
      expect(document.activeElement).toBe(item("Archive"));
      fireEvent.keyDown(item("Archive"), { key: "End" });
      expect(document.activeElement).toBe(item("Inbox"));
      fireEvent.keyDown(item("Inbox"), { key: "ArrowDown" }); // nothing past the end
      expect(document.activeElement).toBe(item("Inbox"));
      fireEvent.keyDown(item("Inbox"), { key: "Home" });
      expect(document.activeElement).toBe(item("Empty"));
      expect(item("Empty").tabIndex).toBe(0);
      expect(outer).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", outer);
    }
  });

  it("opens and enters folders with →, closes them or goes to the parent with ←", () => {
    const { commands } = setup();
    item("Archive").focus();
    fireEvent.keyDown(item("Archive"), { key: "ArrowRight" });
    expect(commands).toEqual([{ kind: "toggle", path: "Projects/Archive" }]);
    fireEvent.keyDown(item("Archive"), { key: "ArrowLeft" }); // a closed folder: to its parent
    expect(document.activeElement).toBe(item("Projects"));
    fireEvent.keyDown(item("Projects"), { key: "ArrowRight" }); // open: into its first child
    expect(document.activeElement).toBe(item("Archive"));
    fireEvent.keyDown(item("Archive"), { key: "ArrowDown" });
    fireEvent.keyDown(item("Orchard plan"), { key: "ArrowLeft" }); // a file: to its parent
    expect(document.activeElement).toBe(item("Projects"));
    fireEvent.keyDown(item("Projects"), { key: "ArrowLeft" }); // open: close it
    expect(commands.at(-1)).toEqual({ kind: "toggle", path: "Projects" });
  });

  it("opens with Enter (Ctrl+Enter in a new tab), toggles a folder with Enter, and renames with F2", () => {
    const { commands } = setup();
    item("Inbox").focus();
    fireEvent.keyDown(item("Inbox"), { key: "Enter" });
    fireEvent.keyDown(item("Inbox"), { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(item("Inbox"), { key: "F2" });
    item("Projects").focus();
    fireEvent.keyDown(item("Projects"), { key: "Enter" });
    expect(commands).toEqual([
      { kind: "open", path: "Inbox.md", click: NONE },
      { kind: "open", path: "Inbox.md", click: { ctrl: true, alt: false, middle: false } },
      { kind: "rename", path: "Inbox.md" },
      { kind: "toggle", path: "Projects" },
    ]);
  });

  it("follows a click with the keyboard's focus", () => {
    setup();
    fireEvent.click(item("Inbox"));
    expect(item("Inbox").tabIndex).toBe(0);
    expect(item("Orchard plan").tabIndex).toBe(-1);
  });

  it("renames in place: the name selected, Enter commits, Escape cancels", async () => {
    const first = setup({ renaming: "Projects/Orchard plan.md" });
    const field = screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement;
    expect(document.activeElement).toBe(field);
    expect(field.value).toBe("Orchard plan");
    expect([field.selectionStart, field.selectionEnd]).toEqual([0, 12]);
    expect(item("Orchard plan").getAttribute("draggable")).toBe("false");
    await first.user.clear(field);
    await first.user.type(field, " Orchard 2027 {Enter}");
    expect(first.commands).toEqual([{ kind: "renamed", path: "Projects/Orchard plan.md", name: "Orchard 2027" }]);
    first.view.unmount();

    const second = setup({ renaming: "Inbox.md" });
    await second.user.keyboard("{Escape}");
    expect(second.commands).toEqual([{ kind: "cancelRename" }]);
  });

  it("refuses a name with a slash: Enter does nothing, and leaving cancels", async () => {
    const { commands, user } = setup({ renaming: "Inbox.md" });
    const field = screen.getByRole("textbox", { name: "Name" });
    await user.type(field, "/x{Enter}");
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(commands).toEqual([]);
    fireEvent.blur(field);
    expect(commands).toEqual([{ kind: "cancelRename" }]);
  });

  it("commits a rename when the field loses focus, once", async () => {
    const { commands, user } = setup({ renaming: "Inbox.md" });
    const field = screen.getByRole("textbox", { name: "Name" });
    await user.type(field, "{End} 2");
    fireEvent.blur(field);
    fireEvent.blur(field);
    expect(commands).toEqual([{ kind: "renamed", path: "Inbox.md", name: "Inbox 2" }]);
  });

  it("asks for a row's context menu, and the root's on the empty space", () => {
    const { commands } = setup();
    fireEvent.contextMenu(item("Projects"), { clientX: 10, clientY: 20 });
    fireEvent.contextMenu(item("Inbox"), { clientX: 11, clientY: 21 });
    fireEvent.contextMenu(screen.getByRole("tree", { name: "Files" }), { clientX: 12, clientY: 22 });
    expect(commands).toEqual([
      { kind: "menu", path: "Projects", folder: true, x: 10, y: 20 },
      { kind: "menu", path: "Inbox.md", folder: false, x: 11, y: 21 },
      { kind: "menu", path: null, folder: true, x: 12, y: 22 },
    ]);
  });

  it("drags a row's path, and drops onto a folder, a file's folder, or the root", () => {
    const { commands } = setup();
    const dt = dataTransfer();
    fireEvent.dragStart(item("Inbox"), { dataTransfer: dt });
    expect(JSON.parse(dt.getData(DRAG_TYPE))).toEqual(["Inbox.md"]);
    expect(dt.getData("text/plain")).toBe("Inbox.md");
    const carried = () => dataTransfer({ [DRAG_TYPE]: JSON.stringify(["Inbox.md"]) });

    const over = carried();
    fireEvent.dragOver(item("Archive"), { dataTransfer: over });
    expect(item("Archive").className).toContain("ring-accent");
    fireEvent.drop(item("Archive"), { dataTransfer: over });
    expect(item("Archive").className).not.toContain("ring-accent");
    fireEvent.drop(item("Seeds"), { dataTransfer: carried() });
    fireEvent.drop(screen.getByRole("tree", { name: "Files" }), { dataTransfer: carried() });
    expect(commands).toEqual([
      { kind: "drop", paths: ["Inbox.md"], folder: "Projects/Archive" },
      { kind: "drop", paths: ["Inbox.md"], folder: "Projects" },
      { kind: "drop", paths: ["Inbox.md"], folder: "" },
    ]);
  });

  it("takes no drop of anything but its own rows", () => {
    const { commands } = setup();
    const foreign = dataTransfer({ "text/plain": "hello" });
    const event = new Event("dragover", { bubbles: true, cancelable: true });
    Object.assign(event, { dataTransfer: foreign });
    item("Archive").dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    fireEvent.drop(item("Archive"), { dataTransfer: foreign });
    expect(commands).toEqual([]);
  });

  it("offers its toolbar's commands, Collapse all while a folder is open", () => {
    const { commands, view } = setup();
    fireEvent.click(screen.getByRole("button", { name: "New note" }));
    fireEvent.click(screen.getByRole("button", { name: "New folder" }));
    fireEvent.click(screen.getByRole("button", { name: "Change sort order" }));
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(commands.map((c) => c.kind)).toEqual(["newNote", "newFolder", "sortMenu", "collapseAll"]);
    view.unmount();
    const closed = setup({ rows: rows([]), anyExpanded: false });
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    expect(closed.commands).toEqual([{ kind: "expandAll" }]);
  });

  it("says an empty vault has no notes and offers one, but not before the listing arrives", () => {
    const { commands, view } = setup({ rows: [] as TreeRow[] });
    expect(screen.getByText("No notes yet")).toBeTruthy();
    expect(screen.queryByRole("tree")).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: "New note" }).at(-1) as HTMLElement);
    expect(commands).toEqual([{ kind: "newNote" }]);
    view.unmount();
    setup({ rows: [], loaded: false });
    expect(screen.queryByText("No notes yet")).toBeNull();
  });
});
