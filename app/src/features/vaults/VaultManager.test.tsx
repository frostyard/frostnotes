// T-0082: the vault manager (docs/specs/ui.md#vault-manager).
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { type VaultEntry, VaultManager, type VaultManagerProps } from "./VaultManager";

const VAULTS: VaultEntry[] = [
  { id: 3, name: "garden", path: "/home/wren/garden" },
  { id: 1, name: "Notes", path: "/home/wren/Notes" },
  { id: 2, name: "Work", path: "/home/wren/Documents/Work" },
];

function setup(props: Partial<VaultManagerProps> = {}) {
  const all: VaultManagerProps = {
    vaults: VAULTS,
    current: 1,
    mode: "dialog",
    canPick: true,
    onOpen: vi.fn(),
    onRemove: vi.fn(),
    onCreate: vi.fn(),
    onOpenFolder: vi.fn(),
    onClose: vi.fn(),
    ...props,
  };
  const view = render(<VaultManager {...all} />);
  return { props: all, user: userEvent.setup(), ...view };
}

const dialog = () => screen.getByRole("dialog", { name: "Manage vaults" });
const rows = () => within(screen.getByRole("list", { name: "Vaults" })).getAllByRole("listitem");

describe("VaultManager", () => {
  it("lists the vaults in order, each with its path, the window's marked", () => {
    setup();
    expect(rows().map((r) => r.textContent)).toEqual([
      "garden/home/wren/garden",
      "Notes/home/wren/Notes",
      "Work/home/wren/Documents/Work",
    ]);
    const current = screen.getByRole("button", { name: /^Notes/ });
    expect(current.getAttribute("aria-current")).toBe("true");
    expect(screen.getByRole("button", { name: /^Work/ }).hasAttribute("aria-current")).toBe(false);
  });

  it("opens a vault with a click on its row", async () => {
    const { props, user } = setup();
    await user.click(screen.getByRole("button", { name: /^Work/ }));
    expect(props.onOpen).toHaveBeenCalledWith(2);
  });

  it("removes a vault from the list through its options", async () => {
    const { props, user } = setup();
    await user.click(screen.getByRole("button", { name: "Options for Work" }));
    const menu = screen.getByRole("menu", { name: "Options for Work" });
    await user.click(within(menu).getByRole("menuitem", { name: "Remove from list" }));
    expect(props.onRemove).toHaveBeenCalledWith(VAULTS[2]);
    expect(props.onOpen).not.toHaveBeenCalled();
  });

  it("opens the same menu from a row's context menu", async () => {
    const { props, user } = setup();
    fireEvent.contextMenu(screen.getByRole("button", { name: /^garden/ }), { clientX: 40, clientY: 60 });
    await user.click(screen.getByRole("menuitem", { name: "Remove from list" }));
    expect(props.onRemove).toHaveBeenCalledWith(VAULTS[0]);
  });

  it("only opens vaults for a caller that may not manage them", async () => {
    const { props, user } = setup({ manage: false });
    expect(screen.queryByRole("button", { name: "Options for Work" })).toBeNull();
    fireEvent.contextMenu(screen.getByRole("button", { name: /^garden/ }), { clientX: 40, clientY: 60 });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.queryByRole("button", { name: "Create" })).toBeNull();
    expect(screen.getByText(/added and removed on the machine that runs notesd/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /^Work/ }));
    expect(props.onOpen).toHaveBeenCalledWith(2);
  });

  it("says there are no vaults yet", () => {
    setup({ vaults: [], current: null, mode: "window" });
    expect(screen.getByText("No vaults yet")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Vaults" })).toBeNull();
  });

  it("offers Create new vault, Open folder as vault and how to join a hub", async () => {
    const { props, user } = setup();
    expect(screen.getByRole("heading", { name: "Frostnotes" })).toBeTruthy();
    const createRow = screen.getByRole("region", { name: "Create new vault" });
    expect(within(createRow).getByText("Make a new vault in a folder you choose.")).toBeTruthy();
    await user.click(within(createRow).getByRole("button", { name: "Create" }));
    expect(props.onCreate).toHaveBeenCalledTimes(1);
    const openRow = screen.getByRole("region", { name: "Open folder as vault" });
    expect(within(openRow).getByText("Choose a folder of Markdown files.")).toBeTruthy();
    expect(within(openRow).queryByRole("textbox")).toBeNull();
    await user.click(within(openRow).getByRole("button", { name: "Open" }));
    expect(props.onOpenFolder).toHaveBeenCalledWith();
    const hubRow = screen.getByRole("region", { name: "Join a vault on a hub" });
    expect(within(hubRow).getByText("notesctl hub join URL NAME").tagName).toBe("CODE");
  });

  it("takes a typed folder where there is no picker", async () => {
    const { props, user } = setup({ canPick: false });
    const openRow = screen.getByRole("region", { name: "Open folder as vault" });
    await user.click(within(openRow).getByRole("button", { name: "Open" }));
    expect(props.onOpenFolder).not.toHaveBeenCalled();
    await user.type(within(openRow).getByRole("textbox", { name: "Folder path" }), "  /home/wren/Recipes {Enter}");
    expect(props.onOpenFolder).toHaveBeenCalledWith("/home/wren/Recipes");
  });

  it("shows the panel in the actions' place", () => {
    setup({ panel: <p>The create form</p> });
    expect(screen.getByText("The create form")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Create new vault" })).toBeNull();
    expect(rows()).toHaveLength(3);
  });

  it("closes as a dialog with Close, Escape and a click outside", async () => {
    const { props, user } = setup();
    expect(dialog().getAttribute("aria-modal")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    const backdrop = dialog().parentElement as HTMLElement;
    fireEvent.pointerDown(backdrop);
    fireEvent.pointerDown(dialog());
    expect(props.onClose).toHaveBeenCalledTimes(3);
  });

  it("fills the window without Close when there is no vault", () => {
    const { props } = setup({ mode: "window", current: null });
    expect(dialog()).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(props.onClose).not.toHaveBeenCalled();
    expect(dialog().className).not.toContain("fixed");
    expect(dialog().parentElement?.className ?? "").not.toContain("fixed");
  });
});
