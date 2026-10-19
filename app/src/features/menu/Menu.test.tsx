import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Pin } from "lucide-react";
import { describe, expect, it, vi } from "vitest";

import { Menu, type MenuItem, type MenuProps } from "./Menu";

const ITEMS: MenuItem[] = [
  { kind: "item", id: "close", label: "Close", keys: "Ctrl + W" },
  { kind: "item", id: "close-others", label: "Close others", disabled: true },
  { kind: "separator" },
  { kind: "item", id: "pin", label: "Pin", icon: <Pin size={16} data-testid="pin-icon" /> },
  {
    kind: "submenu",
    label: "Sort by",
    items: [
      { kind: "item", id: "name", label: "File name", checked: true },
      { kind: "item", id: "mtime", label: "Modified time", checked: false },
    ],
  },
  { kind: "item", id: "split", label: "Split right" },
];

function setup(over: Partial<MenuProps> = {}) {
  const onChoose = vi.fn();
  const onClose = vi.fn();
  const props: MenuProps = { label: "Tab", items: ITEMS, x: 40, y: 60, onChoose, onClose, ...over };
  const view = render(
    <div>
      <button type="button">Outside</button>
      <Menu {...props} />
    </div>,
  );
  return { onChoose, onClose, view, user: userEvent.setup() };
}

const menu = (name = "Tab") => screen.getByRole("menu", { name });
const item = (name: string) => screen.getByRole("menuitem", { name });

describe("Menu", () => {
  it("draws a named menu at its point, in the document's body, and takes focus", () => {
    const { view } = setup();
    const m = menu();
    expect(view.container.contains(m)).toBe(false);
    expect(document.body.contains(m)).toBe(true);
    expect(m.style.left).toBe("40px");
    expect(m.style.top).toBe("60px");
    expect(m.className).toContain("fixed");
    expect(m.className).toContain("shadow-popover");
    expect(document.activeElement).toBe(m);
  });

  it("draws a sheet from the bottom, the whole width, with 44-high items, in a narrow window", () => {
    render(
      <Menu
        label="Sheet"
        items={[{ kind: "item", id: "a", label: "Alpha" }]}
        x={300}
        y={200}
        sheet
        onChoose={() => {}}
        onClose={() => {}}
      />,
    );
    const menu = screen.getByRole("menu", { name: "Sheet" });
    expect(menu.className).toContain("bottom-0");
    expect(menu.className).toContain("inset-x-0");
    expect(menu.style.left).toBe("");
    expect(screen.getByRole("menuitem", { name: "Alpha" }).className).toContain("h-11");
  });

  it("lists items, separators and submenus in order", () => {
    setup();
    const m = menu();
    const entries = [...m.children].map((el) => el.getAttribute("role") ?? (el.tagName === "HR" ? "separator" : null));
    expect(entries).toEqual(["menuitem", "menuitem", "separator", "menuitem", "menuitem", "menuitem"]);
    expect(
      within(m)
        .getAllByRole("menuitem")
        .map((el) => el.textContent),
    ).toEqual(["CloseCtrl + W", "Close others", "Pin", "Sort by", "Split right"]);
    expect(item("Sort by").getAttribute("aria-haspopup")).toBe("menu");
    expect(item("Sort by").getAttribute("aria-expanded")).toBe("false");
    expect(item("Close others").getAttribute("aria-disabled")).toBe("true");
    expect(within(item("Pin")).getByTestId("pin-icon")).toBeTruthy();
  });

  it("chooses an item by click, then closes; a disabled one does nothing", async () => {
    const { onChoose, onClose, user } = setup();
    await user.click(item("Close others"));
    expect(onChoose).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    await user.click(item("Split right"));
    expect(onChoose).toHaveBeenCalledWith("split");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("moves the highlight with the arrows over enabled items, wrapping, and Enter chooses", () => {
    const { onChoose, onClose } = setup();
    const m = menu();
    fireEvent.keyDown(m, { key: "ArrowDown" });
    expect(document.activeElement).toBe(item("Close"));
    expect(item("Close").className).toContain("bg-accent");
    expect(item("Close").className).toContain("text-accent-contrast");
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowDown" });
    expect(document.activeElement).toBe(item("Pin")); // past the disabled item and the separator
    expect(item("Close").className).not.toContain("bg-accent");
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowUp" });
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowUp" });
    expect(document.activeElement).toBe(item("Split right")); // wrapped
    fireEvent.keyDown(document.activeElement as Element, { key: "Enter" });
    expect(onChoose).toHaveBeenCalledWith("split");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("starts at the last item with ArrowUp, and Space chooses", () => {
    const { onChoose } = setup();
    fireEvent.keyDown(menu(), { key: "ArrowUp" });
    expect(document.activeElement).toBe(item("Split right"));
    fireEvent.keyDown(document.activeElement as Element, { key: " " });
    expect(onChoose).toHaveBeenCalledWith("split");
  });

  it("keeps its keys from the window", () => {
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    try {
      setup();
      fireEvent.keyDown(menu(), { key: "ArrowDown" });
      expect(outer).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", outer);
    }
  });

  it("highlights the item under the pointer", () => {
    setup();
    fireEvent.pointerEnter(item("Pin"));
    expect(item("Pin").className).toContain("bg-accent");
    expect(document.activeElement).toBe(item("Pin"));
    fireEvent.pointerEnter(item("Close others"));
    expect(item("Close others").className).not.toContain("bg-accent");
  });

  it("closes on Escape and Tab, and on a press outside it but not inside", () => {
    const first = setup();
    fireEvent.keyDown(menu(), { key: "Escape" });
    expect(first.onClose).toHaveBeenCalledTimes(1);
    first.view.unmount();

    const second = setup();
    fireEvent.keyDown(menu(), { key: "Tab" });
    expect(second.onClose).toHaveBeenCalledTimes(1);
    second.view.unmount();

    const third = setup();
    fireEvent.pointerDown(item("Pin"));
    expect(third.onClose).not.toHaveBeenCalled();
    fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }));
    expect(third.onClose).toHaveBeenCalledTimes(1);
  });

  it("prevents the webview's own context menu", () => {
    setup();
    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    item("Pin").dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it("opens a submenu with ArrowRight, closes it with ArrowLeft, and chooses in it", () => {
    const { onChoose, onClose } = setup();
    fireEvent.keyDown(menu(), { key: "ArrowUp" });
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowUp" });
    expect(document.activeElement).toBe(item("Sort by"));
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowRight" });
    const sub = menu("Sort by");
    expect(item("Sort by").getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(sub);
    const checks = within(sub).getAllByRole("menuitemcheckbox");
    expect(checks.map((c) => [c.textContent, c.getAttribute("aria-checked")])).toEqual([
      ["File name", "true"],
      ["Modified time", "false"],
    ]);

    fireEvent.keyDown(sub, { key: "ArrowLeft" });
    expect(screen.queryByRole("menu", { name: "Sort by" })).toBeNull();
    expect(document.activeElement).toBe(item("Sort by"));
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.keyDown(document.activeElement as Element, { key: "Enter" });
    const again = menu("Sort by");
    fireEvent.keyDown(again, { key: "ArrowDown" });
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowDown" });
    fireEvent.keyDown(document.activeElement as Element, { key: "Enter" });
    expect(onChoose).toHaveBeenCalledWith("mtime");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("opens a submenu under the pointer, and Escape there closes only it", () => {
    const { onClose } = setup();
    fireEvent.pointerEnter(item("Sort by"));
    const sub = menu("Sort by");
    fireEvent.keyDown(sub, { key: "Escape" });
    expect(screen.queryByRole("menu", { name: "Sort by" })).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.pointerEnter(item("Sort by"));
    expect(menu("Sort by")).toBeTruthy();
    fireEvent.pointerEnter(item("Pin"));
    expect(screen.queryByRole("menu", { name: "Sort by" })).toBeNull();
  });

  it("counts a press in a submenu as inside", () => {
    const { onClose } = setup();
    fireEvent.pointerEnter(item("Sort by"));
    fireEvent.pointerDown(screen.getByRole("menuitemcheckbox", { name: "File name" }));
    expect(onClose).not.toHaveBeenCalled();
  });
});
