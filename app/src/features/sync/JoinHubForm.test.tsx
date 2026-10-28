// The join form (docs/specs/ui.md#vault-manager, docs/specs/ui.md#settings):
// the hub's URL and vault name, the code to approve, the wait, and the
// pair-again mode that asks for no name.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { JoinHubForm, type JoinHubFormProps } from "./JoinHubForm";

function setup(props: Partial<JoinHubFormProps> = {}) {
  const all: JoinHubFormProps = {
    mode: "join",
    phase: "form",
    onJoin: vi.fn(),
    onBack: vi.fn(),
    onCancel: vi.fn(),
    ...props,
  };
  const view = render(<JoinHubForm {...all} />);
  return { props: all, user: userEvent.setup(), ...view };
}

const join = () => screen.getByRole("button", { name: "Join" });

describe("JoinHubForm", () => {
  it("enables Join once the hub's URL and the vault's name are typed", async () => {
    const { props, user } = setup();
    expect(join().hasAttribute("disabled")).toBe(true);
    await user.type(screen.getByRole("textbox", { name: "Hub URL" }), "https://lodge.example.ts.net/device/notes");
    expect(join().hasAttribute("disabled")).toBe(true);
    await user.type(screen.getByRole("textbox", { name: "Hub vault name" }), " shared ");
    expect(join().hasAttribute("disabled")).toBe(false);
    await user.type(screen.getByRole("textbox", { name: "This device's name (optional)" }), " laptop ");
    await user.click(join());
    expect(props.onJoin).toHaveBeenCalledWith({
      url: "https://lodge.example.ts.net/device/notes",
      name: "shared",
      device: "laptop",
    });
  });

  it("shows the code to approve, the command that approves it, and Cancel", async () => {
    const { props, user } = setup({ phase: "approve", code: "ABCD-EFGH" });
    expect(screen.getByText("ABCD-EFGH")).toBeTruthy();
    expect(screen.getByText("notesctl hub approve ABCD-EFGH").tagName).toBe("CODE");
    expect(screen.getByText(/The code expires in 10 minutes\./)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(props.onCancel).toHaveBeenCalledTimes(1);
  });

  it("says it is joining, and cancels", async () => {
    const { props, user } = setup({ phase: "joining" });
    expect(screen.getByText("Joining…")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(props.onCancel).toHaveBeenCalledTimes(1);
  });

  it("pairs again with no name field, at the vault's hub", () => {
    setup({ mode: "pair", url: "https://lodge.example.ts.net/device/notes" });
    expect(screen.getByRole("heading", { name: "Pair again" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Hub vault name" })).toBeNull();
    const url = screen.getByRole("textbox", { name: "Hub URL" }) as HTMLInputElement;
    expect(url.value).toBe("https://lodge.example.ts.net/device/notes");
    expect(join().hasAttribute("disabled")).toBe(false);
  });

  it("needs a folder for a new vault", async () => {
    const { user } = setup({ location: { path: null, canPick: false, onBrowse: vi.fn(), onLocation: vi.fn() } });
    await user.type(screen.getByRole("textbox", { name: "Hub URL" }), "https://hub.example");
    await user.type(screen.getByRole("textbox", { name: "Hub vault name" }), "shared");
    expect(screen.getByRole("textbox", { name: "Location" })).toBeTruthy();
    expect(join().hasAttribute("disabled")).toBe(true);
  });

  it("says why the last attempt failed", () => {
    setup({ error: "The pairing expired: the code was not approved within 10 minutes." });
    expect(screen.getByText("The pairing expired: the code was not approved within 10 minutes.")).toBeTruthy();
  });

  it("goes back from the fields", async () => {
    const { props, user } = setup();
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(props.onBack).toHaveBeenCalledTimes(1);
  });
});
