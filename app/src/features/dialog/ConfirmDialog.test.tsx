import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfirmDialog, type ConfirmDialogProps } from "./ConfirmDialog";

function setup(over: Partial<ConfirmDialogProps> = {}) {
  const props: ConfirmDialogProps = {
    title: 'Delete "Garden plan"?',
    text: "You can restore it from its version history.",
    confirmLabel: "Delete",
    danger: true,
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  const view = render(<ConfirmDialog {...props} />);
  return { props, view, user: userEvent.setup() };
}

const dialog = () => screen.getByRole("alertdialog", { name: 'Delete "Garden plan"?' });

describe("ConfirmDialog", () => {
  it("draws a modal alert dialog named by its title and described by its text, in the body", () => {
    const { view } = setup();
    expect(dialog().getAttribute("aria-modal")).toBe("true");
    expect(view.container.contains(dialog())).toBe(false);
    expect(dialog().parentElement?.className).toContain("bg-backdrop");
    const describedBy = dialog().getAttribute("aria-describedby");
    expect(describedBy && document.getElementById(describedBy)?.textContent).toBe(
      "You can restore it from its version history.",
    );
    expect(screen.getByRole("heading", { name: 'Delete "Garden plan"?' })).toBeTruthy();
  });

  it("focuses the confirming button, red for a destructive action", () => {
    setup();
    const ok = screen.getByRole("button", { name: "Delete" });
    expect(document.activeElement).toBe(ok);
    expect(ok.className).toContain("bg-flag-1");
  });

  it("draws an ordinary confirmation in the accent, with its own cancel label", () => {
    setup({ danger: false, confirmLabel: "Update", cancelLabel: "Don't update" });
    expect(screen.getByRole("button", { name: "Update" }).className).toContain("bg-accent");
    expect(screen.getByRole("button", { name: "Don't update" })).toBeTruthy();
  });

  it("confirms with its button and with Enter on it", async () => {
    const { props, user } = setup();
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(props.onConfirm).toHaveBeenCalledTimes(2);
    expect(props.onCancel).not.toHaveBeenCalled();
  });

  it("cancels with Cancel, Escape and a press outside it, keeping its keys from the window", async () => {
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    try {
      const { props, user } = setup();
      await user.click(screen.getByRole("button", { name: "Cancel" }));
      fireEvent.keyDown(dialog(), { key: "Escape" });
      fireEvent.pointerDown(dialog());
      fireEvent.pointerDown(dialog().parentElement as HTMLElement);
      expect(props.onCancel).toHaveBeenCalledTimes(3);
      expect(props.onConfirm).not.toHaveBeenCalled();
      expect(outer).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", outer);
    }
  });

  it("gives focus back when it goes", () => {
    const behind = document.createElement("button");
    document.body.append(behind);
    behind.focus();
    const { view } = setup();
    view.unmount();
    expect(document.activeElement).toBe(behind);
    behind.remove();
  });
});
