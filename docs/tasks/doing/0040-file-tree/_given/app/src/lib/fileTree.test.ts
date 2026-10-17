import { describe, expect, it } from "vitest";

import {
  ancestors,
  extensionOf,
  fileType,
  type SortOrder,
  type TreeFile,
  type TreeOptions,
  treeRows,
} from "./fileTree";

const file = (path: string, modifiedAt = 0): TreeFile => ({ path, modifiedAt });

const opts = (o: Partial<TreeOptions> = {}): TreeOptions => ({
  sort: o.sort ?? "alphabetical",
  expanded: o.expanded ?? new Set(),
  showUnsupported: o.showUnsupported ?? false,
});

// A made-up vault: notes, an image, files Obsidian cannot open, and empty folders.
const FILES = [
  file("Inbox.md", 5),
  file("Seeds.PNG", 9),
  file("README", 1),
  file("archive.tar.gz", 2),
  file("Projects/Garden plan.md", 3),
  file("Projects/Seeds/list.md", 4),
  file("Journal/2026/2026-10-17.md", 7),
  file("Journal/2026/2026-10-16.md", 6),
];
const FOLDERS = ["Archive", "Journal/2026/Drafts"];

/** brief writes a row as "depth kind path", with "+" on an open folder. */
const brief = (rows: ReturnType<typeof treeRows>) =>
  rows.map((r) => `${r.depth} ${r.kind} ${r.path}${r.expanded ? " +" : ""}`);

describe("treeRows", () => {
  it("shows the root's folders, then its files, with every folder closed", () => {
    expect(brief(treeRows(FILES, FOLDERS, opts()))).toEqual([
      "0 folder Archive",
      "0 folder Journal",
      "0 folder Projects",
      "0 file Inbox.md",
      "0 file Seeds.PNG",
    ]);
  });

  it("describes each row", () => {
    const rows = treeRows(FILES, FOLDERS, opts({ expanded: new Set(["Projects"]) }));
    expect(rows.find((r) => r.path === "Projects")).toEqual({
      kind: "folder",
      path: "Projects",
      name: "Projects",
      extension: "",
      type: "folder",
      depth: 0,
      expanded: true,
    });
    expect(rows.find((r) => r.path === "Projects/Garden plan.md")).toEqual({
      kind: "file",
      path: "Projects/Garden plan.md",
      name: "Garden plan",
      extension: "md",
      type: "note",
      depth: 1,
      expanded: false,
    });
    expect(rows.find((r) => r.path === "Seeds.PNG")).toMatchObject({ name: "Seeds", extension: "png", type: "image" });
  });

  it("shows an open folder's folders, then its files, a level deeper", () => {
    const expanded = new Set(["Journal", "Journal/2026", "Projects"]);
    expect(brief(treeRows(FILES, FOLDERS, opts({ expanded })))).toEqual([
      "0 folder Archive",
      "0 folder Journal +",
      "1 folder Journal/2026 +",
      "2 folder Journal/2026/Drafts",
      "2 file Journal/2026/2026-10-16.md",
      "2 file Journal/2026/2026-10-17.md",
      "0 folder Projects +",
      "1 folder Projects/Seeds",
      "1 file Projects/Garden plan.md",
      "0 file Inbox.md",
      "0 file Seeds.PNG",
    ]);
  });

  it("hides what lies under a closed folder, whatever is open below it", () => {
    const expanded = new Set(["Journal/2026", "Projects/Seeds"]);
    expect(brief(treeRows(FILES, FOLDERS, opts({ expanded })))).toEqual([
      "0 folder Archive",
      "0 folder Journal",
      "0 folder Projects",
      "0 file Inbox.md",
      "0 file Seeds.PNG",
    ]);
  });

  it("shows files Obsidian cannot open only when asked", () => {
    expect(brief(treeRows(FILES, FOLDERS, opts({ showUnsupported: true }))).slice(3)).toEqual([
      "0 file archive.tar.gz",
      "0 file Inbox.md",
      "0 file README",
      "0 file Seeds.PNG",
    ]);
    const other = treeRows(FILES, FOLDERS, opts({ showUnsupported: true })).find((r) => r.path === "archive.tar.gz");
    expect(other).toMatchObject({ name: "archive.tar", extension: "gz", type: "other" });
    const bare = treeRows(FILES, FOLDERS, opts({ showUnsupported: true })).find((r) => r.path === "README");
    expect(bare).toMatchObject({ name: "README", extension: "", type: "other" });
  });

  it("keeps a folder whose files are all hidden", () => {
    expect(brief(treeRows([file("Scans/a.tiff")], [], opts()))).toEqual(["0 folder Scans"]);
  });

  it("lists each folder once, however it is known, and keeps dots in folder names", () => {
    const rows = treeRows([file("v1.2/notes.md")], ["v1.2", "v1.2", ""], opts({ expanded: new Set(["v1.2"]) }));
    expect(brief(rows)).toEqual(["0 folder v1.2 +", "1 file v1.2/notes.md"]);
    expect(rows[0]).toMatchObject({ name: "v1.2", extension: "" });
  });

  it("is empty for an empty vault", () => {
    expect(treeRows([], [], opts())).toEqual([]);
  });

  it("builds a large vault's rows", () => {
    // 100 folders of 50 notes, and 5,000 notes in one more folder, all open.
    const many: TreeFile[] = [];
    const expanded = new Set<string>(["Big"]);
    for (let f = 0; f < 100; f++) {
      expanded.add(`F${f}`);
      for (let n = 0; n < 50; n++) many.push(file(`F${f}/Note ${n}.md`, n));
    }
    for (let n = 0; n < 5000; n++) many.push(file(`Big/Day ${n}.md`, n));
    const rows = treeRows(many, [], opts({ expanded }));
    expect(rows).toHaveLength(10_101);
    expect(brief(rows.slice(0, 3))).toEqual(["0 folder Big +", "1 file Big/Day 0.md", "1 file Big/Day 1.md"]);
    expect(brief(rows.slice(-2))).toEqual(["1 file F99/Note 48.md", "1 file F99/Note 49.md"]);
  });
});

describe("treeRows sorting", () => {
  // One folder with two subfolders and files whose names and times test
  // each order: numbers in names, case, and two files of one base name.
  const files = [
    file("S/Note 10.md", 100),
    file("S/Note 2.md", 300),
    file("S/apple.md", 200),
    file("S/Banana.md", 300),
    file("S/banana.png", 50),
    file("S/Zed.md", 10),
  ];
  const folders = ["S/zeta", "S/Alpha"];
  const names = (sort: SortOrder) =>
    treeRows(files, folders, opts({ sort, expanded: new Set(["S"]) }))
      .slice(1)
      .map((r) => r.path.slice(2));

  it("sorts by name as Obsidian does: numbers by value, case ignored, then the whole name", () => {
    expect(names("alphabetical")).toEqual([
      "Alpha",
      "zeta",
      "apple.md",
      "Banana.md",
      "banana.png",
      "Note 2.md",
      "Note 10.md",
      "Zed.md",
    ]);
  });

  it("reverses the files by name, but never the folders", () => {
    expect(names("alphabeticalReverse")).toEqual([
      "Alpha",
      "zeta",
      "Zed.md",
      "Note 10.md",
      "Note 2.md",
      "banana.png",
      "Banana.md",
      "apple.md",
    ]);
  });

  it("sorts by modification time, newest or oldest first, ties by name", () => {
    expect(names("byModifiedTime")).toEqual([
      "Alpha",
      "zeta",
      "Banana.md",
      "Note 2.md",
      "apple.md",
      "Note 10.md",
      "banana.png",
      "Zed.md",
    ]);
    expect(names("byModifiedTimeReverse")).toEqual([
      "Alpha",
      "zeta",
      "Zed.md",
      "banana.png",
      "Note 10.md",
      "apple.md",
      "Banana.md",
      "Note 2.md",
    ]);
  });
});

describe("extensionOf, fileType and ancestors", () => {
  it("takes the extension from the last segment, in lower case", () => {
    expect(extensionOf("a/b/Photo.JPG")).toBe("jpg");
    expect(extensionOf("archive.tar.gz")).toBe("gz");
    expect(extensionOf("README")).toBe("");
    expect(extensionOf("a/.hidden")).toBe("");
    expect(extensionOf("trailing.")).toBe("");
    expect(extensionOf("v1.2/notes")).toBe("");
  });

  it("knows Obsidian's file types", () => {
    expect(fileType("Daily/2026-10-17.md")).toBe("note");
    expect(fileType("Shouting.MD")).toBe("note");
    expect(fileType("board.canvas")).toBe("canvas");
    expect(fileType("tasks.base")).toBe("base");
    expect(fileType("scan.pdf")).toBe("pdf");
    for (const ext of ["bmp", "png", "jpg", "jpeg", "gif", "svg", "webp", "avif"]) {
      expect(fileType(`x.${ext}`), ext).toBe("image");
    }
    for (const ext of ["mp3", "wav", "m4a", "3gp", "flac", "ogg", "oga", "opus"]) {
      expect(fileType(`x.${ext}`), ext).toBe("audio");
    }
    for (const ext of ["mp4", "webm", "ogv", "mov", "mkv"]) {
      expect(fileType(`x.${ext}`), ext).toBe("video");
    }
    for (const path of ["notes.txt", "README", "scan.tiff", "a.md/b"]) {
      expect(fileType(path), path).toBe("other");
    }
  });

  it("lists a path's folders, outermost first", () => {
    expect(ancestors("a/b/c.md")).toEqual(["a", "a/b"]);
    expect(ancestors("a/b")).toEqual(["a"]);
    expect(ancestors("c.md")).toEqual([]);
  });
});
