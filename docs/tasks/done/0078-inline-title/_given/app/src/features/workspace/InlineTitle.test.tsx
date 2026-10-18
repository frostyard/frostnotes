// T-0078's given tests: the inline title renames on Enter or blur,
// restores on Escape, and refuses blank or impossible names.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineTitle } from "./InlineTitle";

function setup(title = "Garden plan") {
  const onRename = vi.fn();
  const onDone = vi.fn();
  const r = render(<InlineTitle title={title} onRename={onRename} onDone={onDone} />);
  const field = () => screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement;
  return {
    onRename,
    onDone,
    field,
    user: userEvent.setup(),
    rerender: (t: string) => r.rerender(<InlineTitle title={t} onRename={onRename} onDone={onDone} />),
  };
}

describe("the inline title", () => {
  it("shows the file's name", () => {
    const { field } = setup();
    expect(field().value).toBe("Garden plan");
  });

  it("renames on Enter, trimmed, and gives the editor its turn", async () => {
    const { field, onRename, onDone, user } = setup();
    await user.clear(field());
    await user.type(field(), "  Orchard plan {Enter}");
    expect(onRename.mock.calls).toEqual([["Orchard plan"]]);
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(document.activeElement).not.toBe(field());
  });

  it("renames when it loses focus", async () => {
    const { field, onRename, user } = setup();
    await user.type(field(), " 2");
    await user.tab();
    expect(onRename.mock.calls).toEqual([["Garden plan 2"]]);
  });

  it("restores the name on Escape, renaming nothing", async () => {
    const { field, onRename, onDone, user } = setup();
    await user.type(field(), " x{Escape}");
    expect(field().value).toBe("Garden plan");
    expect(onRename).not.toHaveBeenCalled();
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it("renames nothing for a blank or an unchanged name", async () => {
    const { field, onRename, user } = setup();
    await user.clear(field());
    await user.type(field(), "   {Enter}");
    expect(field().value).toBe("Garden plan");
    await user.click(field());
    await user.tab();
    expect(onRename).not.toHaveBeenCalled();
  });

  it("refuses a name a file cannot have, and says why", async () => {
    const { field, onRename, user } = setup();
    // "[[" types a "[" in user-event.
    for (const bad of ["a/b", "what?", "x#y", "[[draft]"]) {
      await user.clear(field());
      await user.type(field(), `${bad}{Enter}`);
      expect(field().value).toBe("Garden plan");
    }
    expect(onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toContain("cannot contain");
    await user.type(field(), "!");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("follows a new name from outside", () => {
    const { field, rerender } = setup();
    rerender("Renamed");
    expect(field().value).toBe("Renamed");
  });
});
