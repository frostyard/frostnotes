import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TitleBar, type WindowCommand } from "./TitleBar";

describe("TitleBar", () => {
  it("drags the window and sends window commands", async () => {
    const commands: WindowCommand[] = [];
    render(
      <TitleBar
        title="Frostnotes"
        sidebarWidth={240}
        maximized={false}
        windowControls
        onCommand={(c) => commands.push(c)}
      />,
    );
    const bar = screen.getByRole("toolbar", { name: "Title bar" });
    expect(bar.hasAttribute("data-tauri-drag-region")).toBe(true);
    expect(screen.getByText("Frostnotes")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Toggle Sidebar" }).getAttribute("aria-pressed")).toBe("true");

    const user = userEvent.setup();
    for (const name of ["Toggle Sidebar", "Minimize", "Maximize", "Close"]) {
      await user.click(screen.getByRole("button", { name }));
    }
    expect(commands.map((c) => c.kind)).toEqual(["toggleSidebar", "minimize", "toggleMaximize", "close"]);
  });

  it("offers Restore while maximized", () => {
    render(<TitleBar title="Frostnotes" sidebarWidth={0} maximized windowControls onCommand={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Restore" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Toggle Sidebar" }).getAttribute("aria-pressed")).toBe("false");
  });

  it("leaves window controls to a browser", () => {
    render(
      <TitleBar title="Frostnotes" sidebarWidth={240} maximized={false} windowControls={false} onCommand={vi.fn()} />,
    );
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(screen.getByRole("button", { name: "Toggle Sidebar" })).toBeTruthy();
  });
});
