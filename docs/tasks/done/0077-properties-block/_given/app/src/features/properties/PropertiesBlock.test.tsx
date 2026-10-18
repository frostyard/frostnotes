// T-0077's given tests: the properties block's rows, editors, renames,
// removals, new rows, folding and errors.
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { PropertiesBlock, type PropertiesBlockProps, type PropertyRow } from "./PropertiesBlock";

const ROWS: PropertyRow[] = [
  { key: "status", type: "text", value: "draft" },
  { key: "count", type: "number", value: 3 },
  { key: "done", type: "checkbox", value: false },
  { key: "due", type: "date", value: "2026-10-18" },
  { key: "at", type: "datetime", value: "2026-10-18T09:41" },
  { key: "tags", type: "tags", value: ["garden", "fruit"] },
  { key: "aliases", type: "aliases", value: "Plan" },
  { key: "meta", type: "text", value: { by: "wren" } },
];

function setup(over: Partial<PropertiesBlockProps> = {}) {
  const calls: unknown[][] = [];
  const props: PropertiesBlockProps = {
    rows: ROWS,
    error: null,
    folded: false,
    onFold: (f) => void calls.push(["fold", f]),
    onChange: (k, v) => void calls.push(["change", k, v]),
    onRename: (a, b) => void calls.push(["rename", a, b]),
    onRemove: (k) => void calls.push(["remove", k]),
    ...over,
  };
  const r = render(<PropertiesBlock {...props} />);
  return {
    calls,
    user: userEvent.setup(),
    rerender: (more: Partial<PropertiesBlockProps>) => r.rerender(<PropertiesBlock {...props} {...more} />),
  };
}

const row = (key: string) => screen.getByRole("group", { name: key });

describe("the properties block", () => {
  it("draws a row per key with its type's icon, under a header that folds it", async () => {
    const { calls, user } = setup();
    const header = screen.getByRole("button", { name: "Properties" });
    expect(header.getAttribute("aria-expanded")).toBe("true");
    expect(
      within(screen.getByRole("region", { name: "Properties" }))
        .getAllByRole("group")
        .map((g) => g.getAttribute("aria-label")),
    ).toEqual(ROWS.map((r) => r.key));
    expect(row("due").querySelector("[data-type]")?.getAttribute("data-type")).toBe("date");
    await user.click(header);
    expect(calls).toEqual([["fold", true]]);
  });

  it("hides its rows while folded", () => {
    setup({ folded: true });
    expect(screen.getByRole("button", { name: "Properties" }).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryAllByRole("group")).toHaveLength(0);
    expect(screen.queryByRole("button", { name: "Add property" })).toBeNull();
  });

  it("commits a text on Enter or blur, and restores it on Escape", async () => {
    const { calls, user } = setup();
    const status = within(row("status")).getByRole("textbox", { name: "status" });
    await user.clear(status);
    await user.type(status, "done{Enter}");
    await user.type(status, "x{Escape}");
    expect((status as HTMLInputElement).value).toBe("draft");
    await user.click(status);
    await user.tab();
    expect(calls).toEqual([["change", "status", "done"]]);
  });

  it("commits a number, and nothing for an empty one", async () => {
    const { calls, user } = setup();
    const count = within(row("count")).getByRole("spinbutton", { name: "count" });
    await user.clear(count);
    await user.type(count, "4.5{Enter}");
    await user.clear(count);
    await user.type(count, "{Enter}");
    expect(calls).toEqual([["change", "count", 4.5]]);
  });

  it("checks a checkbox and sets dates", async () => {
    const { calls, user } = setup();
    await user.click(within(row("done")).getByRole("checkbox", { name: "done" }));
    fireEvent.change(within(row("due")).getByLabelText("due"), { target: { value: "2026-11-01" } });
    fireEvent.change(within(row("at")).getByLabelText("at"), { target: { value: "2026-11-01T08:00" } });
    expect(calls).toEqual([
      ["change", "done", true],
      ["change", "due", "2026-11-01"],
      ["change", "at", "2026-11-01T08:00"],
    ]);
  });

  it("draws lists as pills to remove and add, a tag without its #", async () => {
    const { calls, user } = setup();
    const tags = row("tags");
    expect(within(tags).getAllByRole("button", { name: /^Remove (garden|fruit)$/ })).toHaveLength(2);
    await user.click(within(tags).getByRole("button", { name: "Remove fruit" }));
    await user.type(within(tags).getByRole("textbox", { name: "Add to tags" }), "#new{Enter}");
    await user.type(within(row("aliases")).getByRole("textbox", { name: "Add to aliases" }), "Other{Enter}");
    await user.type(within(row("aliases")).getByRole("textbox", { name: "Add to aliases" }), "  {Enter}");
    expect(calls).toEqual([
      ["change", "tags", ["garden"]],
      ["change", "tags", ["garden", "fruit", "new"]],
      ["change", "aliases", ["Plan", "Other"]],
    ]);
  });

  it("shows another value as JSON", () => {
    setup();
    expect((within(row("meta")).getByRole("textbox", { name: "meta" }) as HTMLInputElement).value).toBe(
      '{"by":"wren"}',
    );
  });

  it("renames a key, never to a blank name", async () => {
    const { calls, user } = setup();
    const name = within(row("status")).getByRole("textbox", { name: "Property name" });
    await user.clear(name);
    await user.type(name, "state{Enter}");
    await user.clear(name);
    await user.type(name, " {Enter}");
    expect(calls).toEqual([["rename", "status", "state"]]);
  });

  it("removes a key", async () => {
    const { calls, user } = setup();
    await user.click(screen.getByRole("button", { name: "Remove status" }));
    expect(calls).toEqual([["remove", "status"]]);
  });

  it("adds a key with an empty value, never one taken", async () => {
    const { calls, user } = setup();
    await user.click(screen.getByRole("button", { name: "Add property" }));
    const name = screen.getByRole("textbox", { name: "New property name" });
    expect(document.activeElement).toBe(name);
    await user.type(name, "status{Enter}");
    expect(screen.getByRole("textbox", { name: "New property name" })).toBeTruthy();
    await user.clear(name);
    await user.type(name, "owner{Enter}");
    expect(calls).toEqual([["change", "owner", ""]]);
    expect(screen.queryByRole("textbox", { name: "New property name" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Add property" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("textbox", { name: "New property name" })).toBeNull();
  });

  it("opens its new row, unfolded, when asked", () => {
    const { calls, rerender } = setup({ folded: true, addRequest: 0 });
    expect(screen.queryByRole("textbox", { name: "New property name" })).toBeNull();
    rerender({ folded: false, addRequest: 1 });
    expect(screen.getByRole("textbox", { name: "New property name" })).toBeTruthy();
    expect(calls).toEqual([["fold", false]]);
  });

  it("says why the frontmatter is not properties", () => {
    setup({ rows: [], error: "a list" });
    expect(screen.getByText(/a list/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add property" })).toBeNull();
  });
});
