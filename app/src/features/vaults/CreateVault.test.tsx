// T-0083: the vault manager's create form and a new vault's name
// (docs/specs/ui.md#vault-manager).
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { vaultFolder, vaultNameError } from "../../lib/vaultName";
import { CreateVault, type CreateVaultProps } from "./CreateVault";

describe("vaultNameError", () => {
  it("takes a name, trimmed", () => {
    for (const name of ["Garden", "Work notes", "  Garden  ", "Ünïcödé", "a.b", "notes-2026"]) {
      expect(vaultNameError(name), name).toBeNull();
    }
  });

  it("says what is wrong with a name", () => {
    expect(vaultNameError("")).toBe("A vault needs a name.");
    expect(vaultNameError("   ")).toBe("A vault needs a name.");
    expect(vaultNameError(".hidden")).toBe("A vault's name cannot start with a dot.");
    expect(vaultNameError(" .obsidian")).toBe("A vault's name cannot start with a dot.");
    for (const c of ["\\", "/", ":", "*", "?", '"', "<", ">", "|", "#", "^", "[", "]"]) {
      expect(vaultNameError(`a${c}b`), c).toBe("A vault's name cannot hold any of these: \\ / : * ? \" < > | # ^ [ ]");
    }
  });
});

describe("vaultFolder", () => {
  it("joins a location and a trimmed name", () => {
    expect(vaultFolder("/home/wren/Documents", " Garden ")).toBe("/home/wren/Documents/Garden");
    expect(vaultFolder("/home/wren/Documents/", "Garden")).toBe("/home/wren/Documents/Garden");
    expect(vaultFolder("/", "Garden")).toBe("/Garden");
  });
});

function setup(props: Partial<CreateVaultProps> = {}) {
  const all: CreateVaultProps = {
    location: null,
    canPick: true,
    onBrowse: vi.fn(),
    onLocation: vi.fn(),
    onCreate: vi.fn(),
    onBack: vi.fn(),
    ...props,
  };
  const view = render(<CreateVault {...all} />);
  return { props: all, user: userEvent.setup(), ...view };
}

const create = () => screen.getByRole("button", { name: "Create" });
const name = () => screen.getByRole("textbox", { name: "Vault name" });

describe("CreateVault", () => {
  it("draws the form, the name focused, and no folder yet", () => {
    setup();
    expect(screen.getByRole("region", { name: "Create new vault" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Create new vault" })).toBeTruthy();
    expect(document.activeElement).toBe(name());
    expect(screen.getByText("No folder chosen")).toBeTruthy();
    expect(create().hasAttribute("disabled")).toBe(true);
  });

  it("creates once the name is good and a folder is chosen", async () => {
    const { props, user, rerender } = setup();
    await user.type(name(), "  Garden ");
    expect(create().hasAttribute("disabled")).toBe(true); // no folder yet
    rerender(<CreateVault {...props} location="/home/wren/Documents" />);
    expect(screen.getByText("/home/wren/Documents")).toBeTruthy();
    expect(create().hasAttribute("disabled")).toBe(false);
    await user.click(create());
    expect(props.onCreate).toHaveBeenCalledWith("Garden");
  });

  it("creates with Enter in the name", async () => {
    const { props, user } = setup({ location: "/home/wren" });
    await user.type(name(), "Garden{Enter}");
    expect(props.onCreate).toHaveBeenCalledTimes(1);
    expect(props.onCreate).toHaveBeenCalledWith("Garden");
  });

  it("says what is wrong with the name once typed, and will not create", async () => {
    const { props, user } = setup({ location: "/home/wren" });
    expect(screen.queryByText("A vault needs a name.")).toBeNull();
    await user.type(name(), ".x");
    expect(screen.getByText("A vault's name cannot start with a dot.")).toBeTruthy();
    expect(name().getAttribute("aria-invalid")).toBe("true");
    expect(create().hasAttribute("disabled")).toBe(true);
    await user.type(name(), "{Enter}");
    await user.clear(name());
    expect(screen.getByText("A vault needs a name.")).toBeTruthy();
    expect(props.onCreate).not.toHaveBeenCalled();
  });

  it("browses for the folder with the picker", async () => {
    const { props, user } = setup();
    await user.click(screen.getByRole("button", { name: "Browse" }));
    expect(props.onBrowse).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("textbox", { name: "Location" })).toBeNull();
  });

  it("takes a typed folder where there is no picker", async () => {
    const { props, user, rerender } = setup({ canPick: false });
    expect(screen.queryByRole("button", { name: "Browse" })).toBeNull();
    fireEvent.change(screen.getByRole("textbox", { name: "Location" }), { target: { value: "/home/wren" } });
    expect(props.onLocation).toHaveBeenLastCalledWith("/home/wren");
    rerender(<CreateVault {...props} location="/home/wren" />);
    expect((screen.getByRole("textbox", { name: "Location" }) as HTMLInputElement).value).toBe("/home/wren");
    await user.type(name(), "Garden");
    await user.click(create());
    expect(props.onCreate).toHaveBeenCalledWith("Garden");
  });

  it("goes back", async () => {
    const { props, user } = setup();
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(props.onBack).toHaveBeenCalledTimes(1);
  });
});
