import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type TagNode, tagTree } from "../../lib/tagTree";
import { TagsPane, type TagsPaneProps } from "./TagsPane";

const COUNTS = [
  { tag: "work", count: 3 },
  { tag: "Garden", count: 2 },
  { tag: "garden/fruit", count: 4 },
  { tag: "Garden/Veg/roots", count: 1 },
  { tag: "archive", count: 5 },
];

const shape = (nodes: TagNode[]): unknown[] =>
  nodes.map((n) => (n.children.length ? [n.tag, n.count, shape(n.children)] : [n.tag, n.count]));

describe("tagTree", () => {
  it("nests tags at /, each parent counting its own notes and its children's", () => {
    expect(shape(tagTree(COUNTS, "name"))).toEqual([
      ["archive", 5],
      [
        "Garden",
        7,
        [
          ["Garden/fruit", 4],
          ["Garden/Veg", 1, [["Garden/Veg/roots", 1]]],
        ],
      ],
      ["work", 3],
    ]);
  });

  it("names each node by its last segment, and its tag by its segments, as first spelled", () => {
    const [, garden] = tagTree(COUNTS, "name");
    expect(garden?.name).toBe("Garden");
    expect(garden?.children.map((c) => c.name)).toEqual(["fruit", "Veg"]);
    expect(garden?.children.map((c) => c.tag)).toEqual(["Garden/fruit", "Garden/Veg"]);
  });

  it("sorts by count, most first, ties by name, at every level", () => {
    const counts = [...COUNTS, { tag: "garden/herbs", count: 4 }, { tag: "beans", count: 3 }];
    expect(shape(tagTree(counts, "count"))).toEqual([
      [
        "Garden",
        11,
        [
          ["Garden/fruit", 4],
          ["Garden/herbs", 4],
          ["Garden/Veg", 1, [["Garden/Veg/roots", 1]]],
        ],
      ],
      ["archive", 5],
      ["beans", 3],
      ["work", 3],
    ]);
  });

  it("makes nothing of an empty list", () => {
    expect(tagTree([], "name")).toEqual([]);
  });
});

function setup(over: Partial<TagsPaneProps> = {}) {
  const props: TagsPaneProps = { tags: tagTree(COUNTS, "name"), onSortMenu: vi.fn(), onChoose: vi.fn(), ...over };
  const view = render(<TagsPane {...props} />);
  return { props, view };
}

const items = () => within(screen.getByRole("tree", { name: "Tags" })).getAllByRole("treeitem");

describe("TagsPane", () => {
  it("lists the top tags with their counts, parents folded", () => {
    setup();
    expect(items().map((i) => i.textContent)).toEqual(["archive5", "Garden7", "work3"]);
    expect(items().map((i) => i.getAttribute("aria-expanded"))).toEqual([null, "false", null]);
    expect(screen.getByRole("button", { name: "Garden 7" }).getAttribute("title")).toBe("#Garden");
  });

  it("unfolds and folds a parent with its chevron", () => {
    setup();
    fireEvent.click(screen.getByRole("button", { name: "Expand #Garden" }));
    expect(items().map((i) => i.textContent)).toEqual(["archive5", "Garden7", "fruit4", "Veg1", "work3"]);
    expect(items().map((i) => i.getAttribute("aria-level"))).toEqual(["1", "1", "2", "2", "1"]);
    expect(items()[2]?.style.paddingLeft).toBe("20px");
    fireEvent.click(screen.getByRole("button", { name: "Expand #Garden/Veg" }));
    expect(items().map((i) => i.textContent)).toContain("roots1");
    fireEvent.click(screen.getByRole("button", { name: "Collapse #Garden" }));
    expect(items().map((i) => i.textContent)).toEqual(["archive5", "Garden7", "work3"]);
  });

  it("searches a clicked tag by its whole path", () => {
    const { props } = setup();
    fireEvent.click(screen.getByRole("button", { name: "Expand #Garden" }));
    fireEvent.click(screen.getByRole("button", { name: "fruit 4" }));
    expect(props.onChoose).toHaveBeenCalledWith("Garden/fruit");
  });

  it("opens the sort order's menu at its button", () => {
    const { props } = setup();
    fireEvent.click(screen.getByRole("button", { name: "Change sort order" }));
    expect(props.onSortMenu).toHaveBeenCalledTimes(1);
  });

  it("says when the vault has no tags, and lists nothing while loading", () => {
    const { view } = setup({ tags: [] });
    expect(screen.getByText("No tags found.")).toBeTruthy();
    view.unmount();
    setup({ tags: null });
    expect(screen.queryByRole("tree")).toBeNull();
    expect(screen.queryByText("No tags found.")).toBeNull();
  });
});
