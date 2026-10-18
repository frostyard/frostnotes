import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { PropertyCount } from "../../rpc/gen/api";
import { PropertiesPane, type PropertiesPaneProps, sortProperties, TYPE_ICONS } from "./PropertiesPane";

const PROPS: PropertyCount[] = [
  { key: "status", type: "text", count: 4 },
  { key: "tags", type: "tags", count: 9 },
  { key: "Due", type: "date", count: 4 },
  { key: "rating", type: "", count: 1 },
];

describe("sortProperties", () => {
  it("sorts by name ignoring case, or by count with ties by name, leaving its input be", () => {
    const input = [...PROPS];
    expect(sortProperties(input, "name").map((p) => p.key)).toEqual(["Due", "rating", "status", "tags"]);
    expect(sortProperties(input, "count").map((p) => p.key)).toEqual(["tags", "Due", "status", "rating"]);
    expect(input).toEqual(PROPS);
  });
});

function setup(over: Partial<PropertiesPaneProps> = {}) {
  const props: PropertiesPaneProps = {
    properties: PROPS,
    sort: "name",
    onSortMenu: vi.fn(),
    onChoose: vi.fn(),
    ...over,
  };
  const view = render(<PropertiesPane {...props} />);
  return { props, view };
}

const rows = () => within(screen.getByRole("list", { name: "All properties" })).getAllByRole("button");

describe("PropertiesPane", () => {
  it("lists the keys in the sort order, each with its type and count", () => {
    setup();
    expect(rows().map((r) => r.textContent)).toEqual(["Due4", "rating1", "status4", "tags9"]);
    expect(rows().map((r) => r.getAttribute("data-type"))).toEqual(["date", "text", "text", "tags"]);
    expect(rows().every((r) => r.querySelector("svg") !== null)).toBe(true);
  });

  it("sorts by count when asked", () => {
    setup({ sort: "count" });
    expect(rows().map((r) => r.textContent)).toEqual(["tags9", "Due4", "status4", "rating1"]);
  });

  it("has an icon for each of the vault's property types", () => {
    for (const t of ["text", "multitext", "number", "checkbox", "date", "datetime", "aliases", "tags"]) {
      expect(TYPE_ICONS[t]).toBeTruthy();
    }
  });

  it("lists the notes with a clicked key, and opens the sort menu", () => {
    const { props } = setup();
    fireEvent.click(rows()[2] as HTMLElement);
    expect(props.onChoose).toHaveBeenCalledWith("status");
    fireEvent.click(screen.getByRole("button", { name: "Change sort order" }));
    expect(props.onSortMenu).toHaveBeenCalledTimes(1);
  });

  it("says when the vault has none, and lists nothing while loading", () => {
    const { view } = setup({ properties: [] });
    expect(screen.getByText("No properties found.")).toBeTruthy();
    view.unmount();
    setup({ properties: null });
    expect(screen.queryByRole("list")).toBeNull();
  });
});
