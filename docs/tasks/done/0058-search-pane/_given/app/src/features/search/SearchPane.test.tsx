import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { SearchHit } from "../../rpc/gen/api";
import { SearchPane, type SearchPaneProps } from "./SearchPane";

const HITS: SearchHit[] = [
  {
    path: "Projects/Orchard plan.md",
    title: "Orchard plan",
    snippet: [
      { text: "Prune the old ", match: false },
      { text: "pear", match: true },
      { text: " in March", match: false },
    ],
  },
  { path: "Pears.md", title: "Pears", snippet: [] },
];

function Harness(props: Partial<SearchPaneProps> & { initial?: string }) {
  const [query, setQuery] = useState(props.initial ?? "pear");
  return (
    <SearchPane
      query={query}
      onQuery={(q) => {
        setQuery(q);
        props.onQuery?.(q);
      }}
      results={props.results === undefined ? HITS : props.results}
      error={props.error ?? null}
      focusRequest={props.focusRequest ?? 0}
      onOpen={props.onOpen ?? vi.fn()}
    />
  );
}

const field = () => screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement;
const results = () => screen.getByRole("list", { name: "Results" });

describe("SearchPane", () => {
  it("shows the query and how many notes matched", () => {
    render(<Harness />);
    expect(field().value).toBe("pear");
    expect(screen.getByText("2 results")).toBeTruthy();
  });

  it("groups hits by note: title and folder, then the snippet with its matches marked", () => {
    render(<Harness />);
    const title = within(results()).getByRole("button", { name: "Orchard plan Projects" });
    expect(title.getAttribute("title")).toBe("Projects/Orchard plan.md");
    const snippet = within(results()).getByRole("button", { name: "Prune the old pear in March" });
    const marks = [...snippet.querySelectorAll("mark")];
    expect(marks.map((m) => m.textContent)).toEqual(["pear"]);
    expect(marks[0]?.className).toContain("bg-highlight");
    // A note at the root has no folder; a hit without a snippet has no snippet row.
    expect(within(results()).getByRole("button", { name: "Pears" })).toBeTruthy();
    expect(within(results()).getAllByRole("button").length).toBe(5); // 2 chevrons, 2 titles, 1 snippet
  });

  it("says 1 result in the singular", () => {
    render(<Harness results={[HITS[1] as SearchHit]} />);
    expect(screen.getByText("1 result")).toBeTruthy();
  });

  it("passes every change of the query on, and clears it with its button", async () => {
    const onQuery = vi.fn();
    render(<Harness onQuery={onQuery} />);
    const user = userEvent.setup();
    await user.type(field(), "s");
    expect(onQuery).toHaveBeenLastCalledWith("pears");
    await user.click(screen.getByRole("button", { name: "Clear search" }));
    expect(onQuery).toHaveBeenLastCalledWith("");
    expect(field().value).toBe("");
    expect(document.activeElement).toBe(field());
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
  });

  it("shows nothing below the field for a blank query", () => {
    render(<Harness initial="  " />);
    expect(screen.queryByText(/results?$/)).toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("shows nothing before the first answer, and the error when the search failed", () => {
    const { unmount } = render(<Harness results={null} />);
    expect(screen.queryByRole("list")).toBeNull();
    unmount();
    render(<Harness results={null} error="the index is not ready" />);
    expect(screen.getByText("the index is not ready")).toBeTruthy();
  });

  it("opens a note from its title or its snippet, with the click's modifiers", () => {
    const onOpen = vi.fn();
    render(<Harness onOpen={onOpen} />);
    fireEvent.click(within(results()).getByRole("button", { name: "Orchard plan Projects" }));
    fireEvent.click(within(results()).getByRole("button", { name: "Prune the old pear in March" }), {
      ctrlKey: true,
    });
    expect(onOpen.mock.calls).toEqual([
      ["Projects/Orchard plan.md", { ctrl: false, alt: false, middle: false }],
      ["Projects/Orchard plan.md", { ctrl: true, alt: false, middle: false }],
    ]);
  });

  it("folds a note's snippet with its chevron", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Fold Orchard plan" }));
    expect(screen.queryByRole("button", { name: "Prune the old pear in March" })).toBeNull();
    expect(screen.getByRole("button", { name: "Unfold Orchard plan" }).getAttribute("aria-expanded")).toBe("false");
  });

  it("focuses and selects the field when asked, not on its first draw", () => {
    const view = render(<Harness />);
    expect(document.activeElement).not.toBe(field());
    view.rerender(<Harness focusRequest={1} />);
    expect(document.activeElement).toBe(field());
    expect([field().selectionStart, field().selectionEnd]).toEqual([0, 4]);
  });
});
