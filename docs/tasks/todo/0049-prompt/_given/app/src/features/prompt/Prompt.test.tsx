import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { highlight, Prompt, type PromptMods, type PromptProps } from "./Prompt";

const FRUIT = ["Apple", "Banana", "Cherry"];
const NONE: PromptMods = { ctrl: false, alt: false, shift: false };

function Harness(props: Partial<PromptProps<string>> & { initial?: readonly string[] }) {
  const [query, setQuery] = useState("");
  const items = props.items ?? (props.initial ?? FRUIT).filter((f) => f.toLowerCase().includes(query.toLowerCase()));
  return (
    <div>
      <button type="button">Behind</button>
      <Prompt<string>
        label="Quick switcher"
        placeholder="Find or create a note…"
        query={query}
        onQuery={setQuery}
        items={items}
        itemKey={(s) => s}
        renderItem={(s) => <span>{s}</span>}
        onEnter={props.onEnter ?? vi.fn()}
        onClose={props.onClose ?? vi.fn()}
        instructions={[
          { keys: "↑↓", text: "to navigate" },
          { keys: "↵", text: "to open" },
          { keys: "esc", text: "to dismiss" },
        ]}
        emptyText="No notes found."
      />
    </div>
  );
}

const field = () => screen.getByRole("combobox", { name: "Quick switcher" });
const options = () => within(screen.getByRole("listbox")).queryAllByRole("option");
const selected = () => options().find((o) => o.getAttribute("aria-selected") === "true")?.textContent;

describe("Prompt", () => {
  it("draws a modal dialog in the document's body over a dimmed window, its field focused", () => {
    const view = render(<Harness />);
    const dialog = screen.getByRole("dialog", { name: "Quick switcher" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(view.container.contains(dialog)).toBe(false);
    expect(dialog.className).toContain("bg-window");
    expect(dialog.className).toContain("shadow-popover");
    expect(dialog.parentElement?.className).toContain("bg-backdrop");
    expect(dialog.parentElement?.className).toContain("fixed");
    expect(document.activeElement).toBe(field());
    expect(field().getAttribute("placeholder")).toBe("Find or create a note…");
  });

  it("lists the items as options, the first selected, and the instructions under them", () => {
    render(<Harness />);
    expect(options().map((o) => o.textContent)).toEqual(FRUIT);
    expect(selected()).toBe("Apple");
    expect(options()[0]?.className).toContain("bg-selection-inactive");
    expect(options()[1]?.className).not.toContain("bg-selection-inactive");
    expect(screen.getByText("to navigate").textContent).toBe("↑↓ to navigate");
    expect(screen.getByText("to dismiss")).toBeTruthy();
  });

  it("points the field at the selected option", () => {
    render(<Harness />);
    const first = options()[0] as HTMLElement;
    expect(first.id).not.toBe("");
    expect(field().getAttribute("aria-activedescendant")).toBe(first.id);
    fireEvent.keyDown(field(), { key: "ArrowDown" });
    expect(field().getAttribute("aria-activedescendant")).toBe(options()[1]?.id);
  });

  it("moves the selection with the arrows, wrapping, and keeps the keys from the window", () => {
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    try {
      render(<Harness />);
      fireEvent.keyDown(field(), { key: "ArrowDown" });
      expect(selected()).toBe("Banana");
      fireEvent.keyDown(field(), { key: "ArrowDown" });
      fireEvent.keyDown(field(), { key: "ArrowDown" });
      expect(selected()).toBe("Apple");
      fireEvent.keyDown(field(), { key: "ArrowUp" });
      expect(selected()).toBe("Cherry");
      expect(outer).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", outer);
    }
  });

  it("takes typed queries, and a new list starts at its top", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    fireEvent.keyDown(field(), { key: "ArrowDown" });
    await user.type(field(), "an");
    expect((field() as HTMLInputElement).value).toBe("an");
    expect(options().map((o) => o.textContent)).toEqual(["Banana"]);
    expect(selected()).toBe("Banana");
  });

  it("enters the selected item with the modifiers held", () => {
    const onEnter = vi.fn();
    render(<Harness onEnter={onEnter} />);
    fireEvent.keyDown(field(), { key: "ArrowDown" });
    fireEvent.keyDown(field(), { key: "Enter" });
    fireEvent.keyDown(field(), { key: "Enter", ctrlKey: true, altKey: true });
    fireEvent.keyDown(field(), { key: "Enter", shiftKey: true });
    expect(onEnter.mock.calls).toEqual([
      ["Banana", NONE],
      ["Banana", { ctrl: true, alt: true, shift: false }],
      ["Banana", { ctrl: false, alt: false, shift: true }],
    ]);
  });

  it("enters nothing from an empty list, and says why it is empty", async () => {
    const onEnter = vi.fn();
    render(<Harness onEnter={onEnter} />);
    await userEvent.setup().type(field(), "zz");
    expect(options()).toEqual([]);
    expect(screen.getByText("No notes found.")).toBeTruthy();
    expect(field().hasAttribute("aria-activedescendant")).toBe(false);
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(onEnter.mock.calls).toEqual([[undefined, NONE]]);
  });

  it("selects the row under the pointer and enters a clicked one", () => {
    const onEnter = vi.fn();
    render(<Harness onEnter={onEnter} />);
    fireEvent.mouseMove(options()[2] as HTMLElement);
    expect(selected()).toBe("Cherry");
    fireEvent.click(options()[1] as HTMLElement, { ctrlKey: true });
    expect(onEnter.mock.calls).toEqual([["Banana", { ctrl: true, alt: false, shift: false }]]);
  });

  it("closes on Escape and on a press on the dimmed window, not in the dialog", () => {
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    fireEvent.pointerDown(screen.getByRole("dialog"));
    fireEvent.pointerDown(options()[0] as HTMLElement);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(field(), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.pointerDown(screen.getByRole("dialog").parentElement as HTMLElement);
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

const accents = (el: Element) => [...el.querySelectorAll(".text-accent")].map((s) => s.textContent);

describe("highlight", () => {
  it("wraps the ranges that fall in the text, offset by from", () => {
    const { container } = render(<span>{highlight("Garden plan", [[16, 20]], 9)}</span>);
    expect(container.textContent).toBe("Garden plan");
    expect(accents(container)).toEqual(["plan"]);
    const span = container.querySelector(".text-accent");
    expect(span?.className).toContain("font-semibold");
  });

  it("clips a range to the text and skips ranges outside it", () => {
    const { container } = render(
      <span>
        {highlight(
          "Garden",
          [
            [0, 3],
            [5, 9],
            [20, 22],
          ],
          0,
        )}
      </span>,
    );
    expect(container.textContent).toBe("Garden");
    expect(accents(container)).toEqual(["Gar", "n"]);
  });
});
