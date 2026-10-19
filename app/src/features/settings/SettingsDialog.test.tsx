import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SettingsDialog, type SettingsDialogProps, type SettingsSection } from "./SettingsDialog";

const SECTIONS: SettingsSection[] = [
  {
    id: "editor",
    name: "Editor",
    rows: [
      {
        id: "inlineTitle",
        name: "Show inline title",
        description: "The file's name above the note.",
        control: { kind: "toggle", value: true },
      },
      {
        id: "editingMode",
        name: "Default editing mode",
        control: {
          kind: "select",
          value: "live",
          options: [
            { value: "live", label: "Live Preview" },
            { value: "source", label: "Source mode" },
          ],
        },
      },
      { id: "tabSize", name: "Tab size", control: { kind: "number", value: 4, min: 1, max: 8 } },
      { id: "spellcheck", name: "Spellcheck", control: { kind: "toggle", value: false }, disabled: true },
    ],
  },
  {
    id: "files",
    name: "Files and links",
    rows: [
      {
        id: "newNoteFolder",
        name: "Folder to create new notes in",
        control: { kind: "text", value: "Inbox", placeholder: "Example: folder 1/folder 2" },
      },
    ],
  },
  {
    id: "hotkeys",
    name: "Hotkeys",
    rows: [
      {
        id: "workspace:next-tab",
        name: "Go to next tab",
        control: { kind: "keys", keys: ["Ctrl + Tab", "Ctrl + Page Down"] },
      },
      { id: "sync", name: "Hub", control: { kind: "info", text: "https://hub.example:7443" } },
    ],
  },
];

function setup(over: Partial<SettingsDialogProps> = {}) {
  const props: SettingsDialogProps = {
    sections: SECTIONS,
    section: "editor",
    onSection: vi.fn(),
    onChange: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  const view = render(
    <div>
      <button type="button">Behind</button>
      <SettingsDialog {...props} />
    </div>,
  );
  return { props, view, user: userEvent.setup() };
}

const dialog = () => screen.getByRole("dialog", { name: "Settings" });
const nav = () => screen.getByRole("navigation", { name: "Settings sections" });

describe("SettingsDialog in a narrow window", () => {
  it("lists the sections, then shows one over the list with Back", async () => {
    const { props, user } = setup({ narrow: true });
    expect(within(nav()).getAllByRole("button").length).toBe(SECTIONS.length);
    expect(screen.queryByRole("region")).toBeNull();
    const second = SECTIONS[1] as SettingsSection;
    await user.click(within(nav()).getByRole("button", { name: second.name }));
    expect(props.onSection).toHaveBeenCalledWith(second.id);
    expect(screen.queryByRole("navigation", { name: "Settings sections" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(nav()).toBeTruthy();
  });
});

describe("SettingsDialog", () => {
  it("draws a modal dialog in the document's body over a dimmed window, focused", () => {
    const { view } = setup();
    expect(dialog().getAttribute("aria-modal")).toBe("true");
    expect(view.container.contains(dialog())).toBe(false);
    expect(dialog().parentElement?.className).toContain("bg-backdrop");
    expect(dialog().className).toContain("bg-window");
    expect(document.activeElement).toBe(dialog());
  });

  it("lists the sections, the current one marked, and shows its rows under its name", () => {
    setup();
    const buttons = within(nav()).getAllByRole("button");
    expect(buttons.map((b) => b.textContent)).toEqual(["Editor", "Files and links", "Hotkeys"]);
    expect(buttons.map((b) => b.getAttribute("aria-current"))).toEqual(["page", null, null]);
    const section = screen.getByRole("region", { name: "Editor" });
    expect(within(section).getByRole("heading", { name: "Editor" })).toBeTruthy();
    expect(within(section).getByText("The file's name above the note.")).toBeTruthy();
    expect([...section.querySelectorAll("[data-setting]")].map((r) => r.getAttribute("data-setting"))).toEqual([
      "inlineTitle",
      "editingMode",
      "tabSize",
      "spellcheck",
    ]);
  });

  it("asks for another section", async () => {
    const { props, user } = setup();
    await user.click(within(nav()).getByRole("button", { name: "Hotkeys" }));
    expect(props.onSection).toHaveBeenCalledWith("hotkeys");
  });

  it("falls back to the first section for an unknown one", () => {
    setup({ section: "nope" });
    expect(screen.getByRole("region", { name: "Editor" })).toBeTruthy();
  });

  it("flips a toggle, and leaves a disabled one be", async () => {
    const { props, user } = setup();
    const toggle = screen.getByRole("switch", { name: "Show inline title" });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(toggle.className).toContain("bg-accent");
    await user.click(toggle);
    expect(props.onChange).toHaveBeenCalledWith("inlineTitle", false);
    const off = screen.getByRole("switch", { name: "Spellcheck" });
    expect(off.getAttribute("aria-checked")).toBe("false");
    expect(off.hasAttribute("disabled")).toBe(true);
    await user.click(off);
    expect(props.onChange).toHaveBeenCalledTimes(1);
  });

  it("chooses from a select", async () => {
    const { props, user } = setup();
    const select = screen.getByRole("combobox", { name: "Default editing mode" }) as HTMLSelectElement;
    expect(select.value).toBe("live");
    await user.selectOptions(select, "source");
    expect(props.onChange).toHaveBeenCalledWith("editingMode", "source");
  });

  it("sends a number when left or on Enter, clamped and whole, and puts back what is no number", async () => {
    const { props, user } = setup();
    const field = screen.getByRole("spinbutton", { name: "Tab size" }) as HTMLInputElement;
    await user.clear(field);
    await user.type(field, "12{Enter}");
    expect(props.onChange).toHaveBeenLastCalledWith("tabSize", 8);
    expect(field.value).toBe("8");
    await user.clear(field);
    await user.type(field, "2.4");
    fireEvent.blur(field);
    expect(props.onChange).toHaveBeenLastCalledWith("tabSize", 2);
    await user.clear(field);
    fireEvent.blur(field);
    expect(field.value).toBe("4");
    expect(props.onChange).toHaveBeenCalledTimes(2);
  });

  it("sends a text when it is left or Enter is pressed, and only when it changed", async () => {
    const { props, user } = setup({ section: "files" });
    const field = screen.getByRole("textbox", { name: "Folder to create new notes in" }) as HTMLInputElement;
    expect(field.value).toBe("Inbox");
    expect(field.getAttribute("placeholder")).toBe("Example: folder 1/folder 2");
    fireEvent.blur(field);
    expect(props.onChange).not.toHaveBeenCalled();
    await user.type(field, "/Daily");
    expect(props.onChange).not.toHaveBeenCalled();
    await user.keyboard("{Enter}");
    expect(props.onChange).toHaveBeenCalledWith("newNoteFolder", "Inbox/Daily");
  });

  it("shows keys as caps and read-only text as text", () => {
    setup({ section: "hotkeys" });
    const caps = [...screen.getByRole("region", { name: "Hotkeys" }).querySelectorAll("kbd")].map((k) => k.textContent);
    expect(caps).toEqual(["Ctrl + Tab", "Ctrl + Page Down"]);
    expect(screen.getByText("https://hub.example:7443")).toBeTruthy();
  });

  it("closes on Escape, its Close button and a press outside it, and keeps its keys from the window", async () => {
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    try {
      const { props, user } = setup();
      fireEvent.keyDown(dialog(), { key: "Escape" });
      expect(outer).not.toHaveBeenCalled();
      await user.click(screen.getByRole("button", { name: "Close" }));
      fireEvent.pointerDown(dialog());
      expect(props.onClose).toHaveBeenCalledTimes(2);
      fireEvent.pointerDown(dialog().parentElement as HTMLElement);
      expect(props.onClose).toHaveBeenCalledTimes(3);
    } finally {
      window.removeEventListener("keydown", outer);
    }
  });

  it("gives focus back when it goes", () => {
    const behind = document.createElement("button");
    document.body.append(behind);
    behind.focus();
    const { view } = setup();
    expect(document.activeElement).toBe(dialog());
    view.unmount();
    expect(document.activeElement).toBe(behind);
    behind.remove();
  });
});
