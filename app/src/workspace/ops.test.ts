import { describe, expect, it } from "vitest";

import { emptyWorkspace, groups, MAX_CLOSED, MAX_HISTORY, type Split, type ViewState, type Workspace } from "./model";
import {
  activate,
  activeGroup,
  activeTab,
  applyDelete,
  applyMove,
  back,
  close,
  closeAll,
  closeOthers,
  closeToTheRight,
  cycle,
  findTab,
  forward,
  gotoLastTab,
  gotoTab,
  moveTab,
  open,
  pin,
  reopenClosed,
  resize,
  revealPath,
  selectPane,
  setFileSort,
  setFolderOpen,
  setFoldersOpen,
  setSidebarWidth,
  split,
  toggleSidebar,
} from "./ops";
import { validate } from "./validate";

const note = (path: string): ViewState => ({ type: "note", path, mode: "edit" });
const titles = (ws: Workspace) => activeGroup(ws).tabs.map((t) => (t.view.type === "note" ? t.view.path : t.view.type));
const run = (ws: Workspace, ...ops: ((w: Workspace) => Workspace)[]) => ops.reduce((w, op) => op(w), ws);

/** wsWith opens each path in a new tab of a fresh workspace, so the first tab stays empty. */
function wsWith(...paths: string[]): Workspace {
  return paths.reduce((w, p) => open(w, note(p), "tab"), emptyWorkspace());
}

describe("opening", () => {
  it("opens in the active tab, keeping history, and an empty tab's view is not kept", () => {
    let ws = open(emptyWorkspace(), note("a.md"));
    ws = open(ws, note("b.md"));
    expect(activeTab(ws).view).toEqual(note("b.md"));
    expect(activeTab(ws).back).toEqual([note("a.md")]);
    expect(groups(ws.root)[0]?.tabs).toHaveLength(1);
    expect(ws.recent).toEqual(["b.md", "a.md"]);
  });

  it("opens a pinned tab's link in a new tab after it", () => {
    let ws = wsWith("a.md", "b.md");
    ws = activate(ws, activeGroup(ws).tabs[1]?.id ?? "");
    ws = pin(ws, activeTab(ws).id);
    ws = open(ws, note("c.md"));
    expect(titles(ws)).toEqual(["empty", "a.md", "c.md", "b.md"]);
    expect(activeTab(ws).view).toEqual(note("c.md"));
  });

  it("opens to the right and below as new groups", () => {
    let ws = open(emptyWorkspace(), note("a.md"));
    ws = open(ws, note("b.md"), "right");
    expect(ws.root).toMatchObject({ kind: "split", direction: "row", sizes: [0.5, 0.5] });
    ws = open(ws, note("c.md"), "down");
    const row = ws.root as Split;
    expect(row.children[1]).toMatchObject({ kind: "split", direction: "column" });
    expect(activeTab(ws).view).toEqual(note("c.md"));
    // A third group to the right shares the row with its neighbors.
    ws = activate(ws, groups(ws.root)[0]?.tabs[0]?.id ?? "");
    ws = open(ws, note("d.md"), "right");
    expect((ws.root as Split).children).toHaveLength(3);
    expect((ws.root as Split).sizes).toEqual([0.25, 0.25, 0.5]);
  });

  it("goes back and forward, at most MAX_HISTORY each way", () => {
    let ws = emptyWorkspace();
    for (let i = 0; i <= MAX_HISTORY + 5; i++) ws = open(ws, note(`${i}.md`));
    expect(activeTab(ws).back).toHaveLength(MAX_HISTORY);
    ws = run(ws, back, back);
    expect(activeTab(ws).view).toEqual(note(`${MAX_HISTORY + 3}.md`));
    ws = forward(ws);
    expect(activeTab(ws).view).toEqual(note(`${MAX_HISTORY + 4}.md`));
    ws = open(ws, note("x.md"));
    expect(activeTab(ws).forward).toEqual([]);
    expect(forward(ws)).toBe(ws);
  });
});

describe("activating", () => {
  it("cycles, goes to the nth and the last tab", () => {
    const ws = wsWith("a.md", "b.md", "c.md");
    expect(activeTab(cycle(ws, 1)).view.type).toBe("empty"); // wraps
    expect(activeTab(cycle(ws, -1)).view).toEqual(note("b.md"));
    expect(activeTab(gotoTab(ws, 2)).view).toEqual(note("a.md"));
    expect(gotoTab(ws, 9)).toBe(ws);
    expect(activeTab(gotoLastTab(gotoTab(ws, 1))).view).toEqual(note("c.md"));
  });
});

describe("closing", () => {
  it("activates the tab that takes the closed one's place", () => {
    let ws = gotoTab(wsWith("a.md", "b.md", "c.md"), 2);
    ws = close(ws, activeTab(ws).id);
    expect(titles(ws)).toEqual(["empty", "b.md", "c.md"]);
    expect(activeTab(ws).view).toEqual(note("b.md"));
    ws = close(gotoLastTab(ws), activeTab(gotoLastTab(ws)).id);
    expect(activeTab(ws).view).toEqual(note("b.md"));
  });

  it("leaves an empty tab when the last one closes, and reopens what closed", () => {
    let ws = open(emptyWorkspace(), note("a.md"));
    const id = activeTab(ws).id;
    ws = close(ws, id);
    expect(titles(ws)).toEqual(["empty"]);
    ws = reopenClosed(ws);
    expect(titles(ws)).toEqual(["a.md", "empty"]);
    expect(activeTab(ws).id).toBe(id);
    expect(reopenClosed(ws)).toBe(ws);
  });

  it("keeps the last MAX_CLOSED", () => {
    let ws = emptyWorkspace();
    for (let i = 0; i < MAX_CLOSED + 3; i++)
      ws = close(open(ws, note(`${i}.md`), "tab"), activeTab(open(ws, note(`${i}.md`), "tab")).id);
    expect(ws.closed).toHaveLength(MAX_CLOSED);
  });

  it("collapses a group's split when its last tab closes", () => {
    let ws = open(open(emptyWorkspace(), note("a.md")), note("b.md"), "right");
    ws = close(ws, activeTab(ws).id);
    expect(ws.root.kind).toBe("tabs");
    expect(activeTab(ws).view).toEqual(note("a.md"));
    // Reopening a tab whose group is gone puts it after the active tab.
    ws = reopenClosed(ws);
    expect(titles(ws)).toEqual(["a.md", "b.md"]);
  });

  it("closes others, to the right and all, sparing pinned tabs", () => {
    const ws = wsWith("a.md", "b.md", "c.md", "d.md");
    const [, a, b, c] = activeGroup(ws).tabs;
    const pinned = pin(ws, c?.id ?? "");
    expect(titles(closeOthers(pinned, b?.id ?? ""))).toEqual(["b.md", "c.md"]);
    expect(titles(closeToTheRight(pinned, a?.id ?? ""))).toEqual(["empty", "a.md", "c.md"]);
    expect(titles(closeAll(pinned, ws.activeGroup))).toEqual(["c.md"]);
    expect(titles(closeAll(ws, ws.activeGroup))).toEqual(["empty"]);
  });
});

describe("splitting and moving", () => {
  it("splits a tab's view into a new group", () => {
    let ws = open(emptyWorkspace(), note("a.md"));
    ws = split(ws, activeTab(ws).id, "down");
    expect(ws.root).toMatchObject({ kind: "split", direction: "column" });
    expect(groups(ws.root).map((g) => g.tabs[0]?.view)).toEqual([note("a.md"), note("a.md")]);
    expect(ws.activeGroup).toBe(groups(ws.root)[1]?.id);
  });

  it("moves a tab within its group", () => {
    let ws = wsWith("a.md", "b.md", "c.md");
    const a = activeGroup(ws).tabs[1]?.id ?? "";
    ws = moveTab(ws, a, { group: ws.activeGroup, index: 4 });
    expect(titles(ws)).toEqual(["empty", "b.md", "c.md", "a.md"]);
    expect(activeTab(ws).id).toBe(a);
    ws = moveTab(ws, a, { group: ws.activeGroup, index: 0 });
    expect(titles(ws)).toEqual(["a.md", "empty", "b.md", "c.md"]);
  });

  it("moves a tab to another group, collapsing the one it leaves", () => {
    let ws = open(open(emptyWorkspace(), note("a.md")), note("b.md"), "right");
    const [left, right] = groups(ws.root);
    ws = moveTab(ws, right?.tabs[0]?.id ?? "", { group: left?.id ?? "", index: 0 });
    expect(ws.root.kind).toBe("tabs");
    expect(titles(ws)).toEqual(["b.md", "a.md"]);
  });

  it("moves a tab to a group's edge as a new split, and not its group's only tab to its own edge", () => {
    let ws = wsWith("a.md", "b.md");
    const b = activeTab(ws).id;
    ws = moveTab(ws, b, { group: ws.activeGroup, edge: "left" });
    expect(ws.root).toMatchObject({ kind: "split", direction: "row" });
    expect(groups(ws.root).map((g) => g.tabs.map((t) => t.view.type === "note" && t.view.path))).toEqual([
      ["b.md"],
      [false, "a.md"],
    ]);
    expect(moveTab(ws, b, { group: ws.activeGroup, edge: "bottom" })).toBe(ws);
  });

  it("resizes a split, normalized", () => {
    const ws = open(open(emptyWorkspace(), note("a.md")), note("b.md"), "right");
    const sid = ws.root.id;
    expect((resize(ws, sid, [3, 1]).root as Split).sizes).toEqual([0.75, 0.25]);
    expect((resize(ws, sid, [1, 0]).root as Split).sizes[1]).toBeCloseTo(0.05 / 1.05);
    expect(resize(ws, sid, [1]).root).toEqual(ws.root);
  });
});

describe("sidebars", () => {
  it("toggle, size within bounds and select panes", () => {
    let ws = toggleSidebar(emptyWorkspace(), "left");
    expect(ws.left.open).toBe(false);
    ws = selectPane(ws, "left", "search");
    expect(ws.left).toMatchObject({ open: true, active: 1 });
    expect(setSidebarWidth(ws, "right", 50).right.width).toBe(200);
    expect(setSidebarWidth(ws, "right", 9000).right.width).toBe(600);
    expect(selectPane(ws, "left", "outline")).toBe(ws);
  });
});

describe("following the vault", () => {
  it("follows a move of a file or a folder everywhere", () => {
    let ws = open(open(emptyWorkspace(), note("Old/a.md")), note("Old/Deep/b.md"));
    ws = close(open(ws, note("Old/c.md"), "tab"), activeTab(open(ws, note("Old/c.md"), "tab")).id);
    ws = applyMove(ws, "Old", "New");
    expect(activeTab(ws).view).toEqual(note("New/Deep/b.md"));
    expect(activeTab(ws).back).toEqual([note("New/a.md")]);
    expect(ws.closed[0]?.tab.view).toEqual(note("New/c.md"));
    expect(ws.recent).toEqual(["New/c.md", "New/Deep/b.md", "New/a.md"]);
    expect(applyMove(ws, "Older", "x").root).toEqual(ws.root);
  });

  it("forgets a deleted file in history, keeping the tab that shows it", () => {
    let ws = open(open(emptyWorkspace(), note("a.md")), note("b.md"));
    ws = applyDelete(ws, "a.md");
    expect(activeTab(ws).back).toEqual([]);
    expect(ws.recent).toEqual(["b.md"]);
    expect(activeTab(applyDelete(ws, "b.md")).view).toEqual(note("b.md"));
  });
});

describe("validate", () => {
  it("keeps a workspace it wrote", () => {
    let ws = open(open(wsWith("a.md", "b.md"), note("c.md"), "right"), note("d.md"), "down");
    ws = close(pin(ws, activeTab(ws).id), findTab(ws, groups(ws.root)[0]?.tabs[0]?.id ?? "")?.tab.id ?? "");
    expect(validate(JSON.parse(JSON.stringify(ws)))).toEqual(ws);
  });

  it("drops what it does not understand", () => {
    expect(validate(null)).toEqual(emptyWorkspace());
    expect(validate({ version: 2 })).toEqual(emptyWorkspace());
    const ws = validate({
      version: 1,
      root: {
        kind: "split",
        id: "s1",
        direction: "row",
        sizes: [1, "x"],
        children: [
          {
            kind: "tabs",
            id: "g2",
            active: 7,
            tabs: [{ id: "t3", view: { type: "note", path: "a.md", mode: "weird" } }],
          },
          { kind: "tabs", id: "g4", tabs: [{ id: "t3", view: { type: "empty" } }] }, // duplicate tab ID: the group is empty, so it goes
          { kind: "pane", id: "x9" },
        ],
      },
      activeGroup: "g9",
      left: { panes: ["search", "nope"], active: 0, width: 5, open: false },
      right: "garbage",
      recent: ["a.md", "/abs", 3, "a.md"],
      nextId: 1,
    });
    expect(ws.root).toEqual({
      kind: "tabs",
      id: "g2",
      active: 0,
      tabs: [{ id: "t3", view: note("a.md"), back: [], forward: [], pinned: false }],
    });
    expect(ws.activeGroup).toBe("g2");
    expect(ws.left).toEqual({ panes: ["files", "search"], active: 1, width: 200, open: false });
    expect(ws.right.open).toBe(true);
    expect(ws.recent).toEqual(["a.md"]);
    expect(ws.nextId).toBe(4);
  });
});

describe("the Files pane's state", () => {
  it("sorts, opens and closes folders, and reveals a path's folders", () => {
    let ws = emptyWorkspace();
    expect(ws.files).toEqual({ sort: "alphabetical", expanded: [] });
    ws = setFileSort(ws, "byModifiedTime");
    ws = setFolderOpen(ws, "Projects", true);
    ws = setFolderOpen(ws, "Projects", true);
    expect(ws.files).toEqual({ sort: "byModifiedTime", expanded: ["Projects"] });
    ws = revealPath(ws, "Journal/2026/Oct/a.md");
    expect(ws.files.expanded).toEqual(["Projects", "Journal", "Journal/2026", "Journal/2026/Oct"]);
    ws = setFolderOpen(ws, "Projects", false);
    expect(ws.files.expanded).toEqual(["Journal", "Journal/2026", "Journal/2026/Oct"]);
    expect(setFoldersOpen(ws, []).files.expanded).toEqual([]);
  });

  it("follows a folder's move, and validates what was stored", () => {
    let ws = setFoldersOpen(emptyWorkspace(), ["Journal", "Journal/2026", "Other"]);
    ws = applyMove(ws, "Journal", "Diary");
    expect(ws.files.expanded).toEqual(["Diary", "Diary/2026", "Other"]);
    const stored = validate({ ...ws, files: { sort: "sideways", expanded: ["a", "/abs", 3, "a"] } });
    expect(stored.files).toEqual({ sort: "alphabetical", expanded: ["a"] });
    expect(validate({ ...ws, files: undefined }).files).toEqual({ sort: "alphabetical", expanded: [] });
  });
});
