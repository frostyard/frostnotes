import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TitleBar } from "./TitleBar";

describe("TitleBar", () => {
  it("shows the title and toggles the sidebar", async () => {
    const toggle = vi.fn();
    render(<TitleBar title="Frostnotes" sidebarWidth={240} onToggleSidebar={toggle} />);
    expect(screen.getByRole("toolbar", { name: "Title bar" })).toBeTruthy();
    expect(screen.getByText("Frostnotes")).toBeTruthy();
    const button = screen.getByRole("button", { name: "Toggle Sidebar" });
    expect(button.getAttribute("aria-pressed")).toBe("true");
    await userEvent.setup().click(button);
    expect(toggle).toHaveBeenCalledOnce();
  });

  it("shows the sidebar hidden", () => {
    render(<TitleBar title="Frostnotes" sidebarWidth={0} onToggleSidebar={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Toggle Sidebar" }).getAttribute("aria-pressed")).toBe("false");
  });
});
