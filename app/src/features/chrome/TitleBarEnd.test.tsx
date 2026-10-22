import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WindowCommand } from "./TitleBar";
import { TitleBarEnd } from "./TitleBarEnd";

describe("TitleBarEnd", () => {
  it("toggles the right sidebar and sends window commands", async () => {
    const commands: WindowCommand[] = [];
    const toggle = vi.fn();
    render(
      <TitleBarEnd
        rightOpen
        maximized={false}
        windowControls
        onToggleRight={toggle}
        onWindow={(c) => commands.push(c)}
      />,
    );
    expect(screen.getByRole("button", { name: "Toggle right sidebar" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("group", { name: "Window controls" })).toBeTruthy();
    const user = userEvent.setup();
    for (const name of ["Toggle right sidebar", "Minimize", "Maximize", "Close"]) {
      await user.click(screen.getByRole("button", { name }));
    }
    expect(toggle).toHaveBeenCalledOnce();
    expect(commands.map((c) => c.kind)).toEqual(["minimize", "toggleMaximize", "close"]);
  });

  it("offers Restore while maximized", () => {
    render(<TitleBarEnd rightOpen={false} maximized windowControls onToggleRight={vi.fn()} onWindow={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Restore" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Toggle right sidebar" }).getAttribute("aria-pressed")).toBe("false");
  });

  it("leaves window controls to a browser, and the toggle to a window with sidebars", () => {
    render(<TitleBarEnd rightOpen={false} maximized={false} windowControls={false} onWindow={vi.fn()} />);
    expect(screen.queryByRole("group", { name: "Window controls" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Toggle right sidebar" })).toBeNull();
  });
});
