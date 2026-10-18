import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { PaletteCommand } from "../../lib/palette";
import { CommandPalette, type CommandPaletteProps } from "./CommandPalette";

const COMMANDS: PaletteCommand[] = [
  { id: "workspace:new-tab", name: "New tab", keys: ["Ctrl + T"] },
  { id: "switcher:open", name: "Open quick switcher", keys: ["Ctrl + O"] },
  { id: "workspace:next-tab", name: "Go to next tab", keys: ["Ctrl + Tab", "Ctrl + Page Down"] },
  { id: "app:reset-layout", name: "Reset workspace layout", keys: [] },
];

function setup(over: Partial<CommandPaletteProps> = {}) {
  const props: CommandPaletteProps = {
    commands: COMMANDS,
    recent: ["switcher:open"],
    onRun: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(<CommandPalette {...props} />);
  return { props, user: userEvent.setup() };
}

const field = () => screen.getByRole("combobox", { name: "Command palette" });
const options = () => within(screen.getByRole("listbox")).queryAllByRole("option");

describe("CommandPalette", () => {
  it("opens as the command palette, the last command run first, then the rest by name", () => {
    setup();
    expect(screen.getByRole("dialog", { name: "Command palette" })).toBeTruthy();
    expect(field().getAttribute("placeholder")).toBe("Select a command…");
    expect(options().map((o) => o.textContent)).toEqual([
      "Open quick switcherCtrl + O",
      "Go to next tabCtrl + TabCtrl + Page Down",
      "New tabCtrl + T",
      "Reset workspace layout",
    ]);
  });

  it("draws each binding as a key cap", () => {
    setup();
    const row = options()[1] as HTMLElement;
    const caps = [...row.querySelectorAll("kbd")];
    expect(caps.map((c) => c.textContent)).toEqual(["Ctrl + Tab", "Ctrl + Page Down"]);
    expect(caps[0]?.className).toContain("bg-badge");
    expect(caps[0]?.className).toContain("text-badge-text");
    expect(caps[0]?.className).toContain("text-keycap");
  });

  it("matches as typed and highlights the matches", async () => {
    const { user } = setup();
    await user.type(field(), "tab");
    expect(options().map((o) => o.textContent)).toEqual([
      "New tabCtrl + T",
      "Go to next tabCtrl + TabCtrl + Page Down",
    ]);
    const marks = [...(options()[0] as HTMLElement).querySelectorAll(".text-accent")].map((m) => m.textContent);
    expect(marks).toEqual(["tab"]);
  });

  it("closes, then runs the chosen command", async () => {
    const order: string[] = [];
    const onRun = vi.fn((id: string) => order.push(`run ${id}`));
    const onClose = vi.fn(() => order.push("close"));
    const { user } = setup({ onRun, onClose });
    await user.type(field(), "new");
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(order).toEqual(["close", "run workspace:new-tab"]);
  });

  it("runs a clicked command", () => {
    const onRun = vi.fn();
    setup({ onRun });
    fireEvent.click(options()[3] as HTMLElement);
    expect(onRun).toHaveBeenCalledWith("app:reset-layout");
  });

  it("runs nothing when nothing matches, and says so", async () => {
    const onRun = vi.fn();
    const onClose = vi.fn();
    const { user } = setup({ onRun, onClose });
    await user.type(field(), "zebra");
    expect(screen.getByText("No commands found.")).toBeTruthy();
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(onRun).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("lists its keys in the footer", () => {
    setup();
    for (const t of ["to navigate", "to use", "to dismiss"]) expect(screen.getByText(t)).toBeTruthy();
  });
});
