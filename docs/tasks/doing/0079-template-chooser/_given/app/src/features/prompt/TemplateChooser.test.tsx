// T-0079's given tests: the template chooser's results and prompt.
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { templateName, templateResults } from "../../lib/templates";
import { TemplateChooser, type TemplateChooserProps } from "./TemplateChooser";

const TEMPLATES = [
  "Templates/meeting.md",
  "Templates/Daily.md",
  "Templates/Work/Weekly review.md",
  "Templates/Daily.md",
];

describe("templateResults", () => {
  it("lists every template once by name, ignoring case, for a blank query", () => {
    for (const q of ["", "  "]) {
      expect(templateResults(TEMPLATES, q)).toEqual([
        { path: "Templates/Daily.md", name: "Daily", match: null },
        { path: "Templates/meeting.md", name: "meeting", match: null },
        { path: "Templates/Work/Weekly review.md", name: "Weekly review", match: null },
      ]);
    }
  });

  it("matches names, best first, with the spans in the name", () => {
    const r = templateResults(TEMPLATES, "rev");
    expect(r.map((x) => x.name)).toEqual(["Weekly review"]);
    expect(r[0]?.match?.matches).toEqual([[7, 10]]);
    expect(templateResults(TEMPLATES, "work")).toEqual([]);
  });

  it("names a template by its file name without .md", () => {
    expect(templateName("Templates/Work/Weekly review.md")).toBe("Weekly review");
    expect(templateName("Plain.MD")).toBe("Plain");
  });
});

function setup(over: Partial<TemplateChooserProps> = {}) {
  const props: TemplateChooserProps = { templates: TEMPLATES, onChoose: vi.fn(), onClose: vi.fn(), ...over };
  render(<TemplateChooser {...props} />);
  return { props, user: userEvent.setup() };
}

const field = () => screen.getByRole("combobox", { name: "Insert template" });
const options = () => within(screen.getByRole("listbox")).queryAllByRole("option");

describe("TemplateChooser", () => {
  it("opens as Insert template, listing the templates by name", () => {
    setup();
    expect(screen.getByRole("dialog", { name: "Insert template" })).toBeTruthy();
    expect(field().getAttribute("placeholder")).toBe("Choose a template…");
    expect(options().map((o) => o.textContent)).toEqual(["Daily", "meeting", "Weekly review"]);
  });

  it("matches as typed, highlighting, and inserts the chosen template after closing", async () => {
    const order: string[] = [];
    const { user } = setup({
      onChoose: vi.fn((p: string) => order.push(`choose ${p}`)),
      onClose: vi.fn(() => order.push("close")),
    });
    await user.type(field(), "meet");
    expect(options().map((o) => o.textContent)).toEqual(["meeting"]);
    expect([...(options()[0] as HTMLElement).querySelectorAll(".text-accent")].map((m) => m.textContent)).toEqual([
      "meet",
    ]);
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(order).toEqual(["close", "choose Templates/meeting.md"]);
  });

  it("chooses nothing when nothing matches, and says so", async () => {
    const { props, user } = setup();
    await user.type(field(), "zebra");
    expect(screen.getByText("No templates found.")).toBeTruthy();
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(props.onChoose).not.toHaveBeenCalled();
  });

  it("lists its keys in the footer", () => {
    setup();
    for (const t of ["to navigate", "to insert", "to dismiss"]) expect(screen.getByText(t)).toBeTruthy();
  });
});
