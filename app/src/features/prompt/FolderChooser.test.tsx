import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { folderResults, folderText } from "../../lib/folders";
import { FolderChooser, type FolderChooserProps } from "./FolderChooser";

const FOLDERS = ["Projects/Archive", "Journal", "Projects", "journal/2026", "Projects"];

describe("folderResults", () => {
  it("lists the root, then every folder once by path, for a blank query", () => {
    for (const q of ["", "  "]) {
      expect(folderResults(FOLDERS, q)).toEqual([
        { path: "", match: null },
        { path: "Journal", match: null },
        { path: "journal/2026", match: null },
        { path: "Projects", match: null },
        { path: "Projects/Archive", match: null },
      ]);
    }
  });

  it("matches folders by name first, then by path, best first", () => {
    const r = folderResults(FOLDERS, "arch");
    expect(r.map((x) => x.path)).toEqual(["Projects/Archive"]);
    expect(r[0]?.match?.matches).toEqual([[9, 13]]);
    expect(folderResults(FOLDERS, "proj").map((x) => x.path)).toEqual(["Projects", "Projects/Archive"]);
  });

  it("offers the root for /", () => {
    expect(folderResults(FOLDERS, "/")[0]).toEqual({
      path: "",
      match: { score: expect.any(Number), matches: [[0, 1]] },
    });
  });

  it("writes the root as /", () => {
    expect(folderText("")).toBe("/");
    expect(folderText("Journal")).toBe("Journal");
  });
});

function setup(over: Partial<FolderChooserProps> = {}) {
  const props: FolderChooserProps = {
    folders: FOLDERS,
    placeholder: 'Move "Garden plan" to…',
    onChoose: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(<FolderChooser {...props} />);
  return { props, user: userEvent.setup() };
}

const field = () => screen.getByRole("combobox", { name: "Move to folder" });
const options = () => within(screen.getByRole("listbox")).queryAllByRole("option");

describe("FolderChooser", () => {
  it("opens as Move to folder, listing the root and the folders", () => {
    setup();
    expect(screen.getByRole("dialog", { name: "Move to folder" })).toBeTruthy();
    expect(field().getAttribute("placeholder")).toBe('Move "Garden plan" to…');
    expect(options().map((o) => o.textContent)).toEqual([
      "/",
      "Journal",
      "journal/2026",
      "Projects",
      "Projects/Archive",
    ]);
  });

  it("matches as typed, highlighting, and moves to the chosen folder after closing", async () => {
    const order: string[] = [];
    const onChoose = vi.fn((f: string) => order.push(`choose ${f}`));
    const onClose = vi.fn(() => order.push("close"));
    const { user } = setup({ onChoose, onClose });
    await user.type(field(), "arch");
    expect(options().map((o) => o.textContent)).toEqual(["Projects/Archive"]);
    expect([...(options()[0] as HTMLElement).querySelectorAll(".text-accent")].map((m) => m.textContent)).toEqual([
      "Arch",
    ]);
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(order).toEqual(["close", "choose Projects/Archive"]);
  });

  it("moves to the root from its row", () => {
    const { props } = setup();
    fireEvent.click(options()[0] as HTMLElement);
    expect(props.onChoose).toHaveBeenCalledWith("");
  });

  it("moves nowhere when nothing matches, and says so", async () => {
    const { props, user } = setup();
    await user.type(field(), "zebra");
    expect(screen.getByText("No folders found.")).toBeTruthy();
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(props.onChoose).not.toHaveBeenCalled();
    expect(props.onClose).not.toHaveBeenCalled();
  });

  it("lists its keys in the footer", () => {
    setup();
    for (const t of ["to navigate", "to move", "to dismiss"]) expect(screen.getByText(t)).toBeTruthy();
  });
});
