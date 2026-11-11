import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TitleBarEnd } from "./TitleBarEnd";

describe("TitleBarEnd", () => {
  it("toggles the right sidebar", async () => {
    const toggle = vi.fn();
    render(<TitleBarEnd rightOpen onToggleRight={toggle} />);
    const button = screen.getByRole("button", { name: "Toggle right sidebar" });
    expect(button.getAttribute("aria-pressed")).toBe("true");
    await userEvent.setup().click(button);
    expect(toggle).toHaveBeenCalledOnce();
  });

  it("leaves the toggle to a window with sidebars", () => {
    render(<TitleBarEnd rightOpen={false} />);
    expect(screen.queryByRole("button", { name: "Toggle right sidebar" })).toBeNull();
  });
});
