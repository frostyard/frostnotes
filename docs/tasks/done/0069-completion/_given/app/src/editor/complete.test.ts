// T-0069's given tests: link and tag completion. In a test's text, "@" is
// the cursor.
import { CompletionContext, type CompletionResult, type CompletionSource } from "@codemirror/autocomplete";
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { COMPLETION_LIMIT, type CompletionCatalog, type LinkFile, linkCompletion, tagCompletion } from "./complete";
import { markdownSupport } from "./syntax/index";

const FILES: LinkFile[] = [
  { path: "Orchard/North Block/Soil.md", aliases: ["Dirt"] },
  { path: "Orchard/Plan.md", aliases: [] },
  { path: "Plan.md", aliases: ["Roadmap"] },
  { path: "Recipes/Apple Butter.md", aliases: [] },
  { path: "media/soil-map.png", aliases: [] },
];

const HEADINGS = [
  { level: 1, text: "Plan" },
  { level: 2, text: "Goals" },
  { level: 2, text: "Tasks" },
];

function catalog(files: readonly LinkFile[] = FILES): CompletionCatalog & { asked: string[] } {
  const asked: string[] = [];
  return {
    asked,
    files: () => files,
    // The test's link format: the path without ".md", and a short name for the note in the root.
    linkText: (path) => (path === "Plan.md" ? "Plan" : path.replace(/\.md$/, "")),
    headings: async (target) => {
      asked.push(`headings:${target}`);
      return target === "Plan" || target === "" ? HEADINGS : [];
    },
    blocks: async (target) => {
      asked.push(`blocks:${target}`);
      return target === "Soil"
        ? [
            { id: "ph-test", text: "pH 6.5 ^ph-test" },
            { id: "drainage", text: "Drains well ^drainage" },
          ]
        : [];
    },
    tags: () => [
      { name: "harvest", count: 3 },
      { name: "orchard/north", count: 5 },
      { name: "todo", count: 1 },
      { name: "hardy", count: 5 },
    ],
  };
}

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

/** complete runs source with the cursor at text's "@". */
async function complete(source: CompletionSource, text: string): Promise<CompletionResult | null> {
  const pos = text.indexOf("@");
  view?.destroy();
  view = new EditorView({
    parent: document.body,
    state: EditorState.create({
      doc: text.replace("@", ""),
      selection: { anchor: pos },
      extensions: markdownSupport(),
    }),
  });
  ensureSyntaxTree(view.state, view.state.doc.length, 10_000);
  return await source(new CompletionContext(view.state, pos, false));
}

const labels = (r: CompletionResult | null) => r?.options.map((o) => o.label) ?? null;

/** accept applies the option labelled label and shows the text with its cursor. */
function accept(result: CompletionResult | null, label: string): string {
  const v = view as EditorView;
  const option = result?.options.find((o) => o.label === label);
  if (!result || !option) throw new Error(`no option ${label}`);
  const to = result.to ?? v.state.selection.main.head;
  if (typeof option.apply === "function") option.apply(v, option, result.from, to);
  else
    v.dispatch({
      changes: { from: result.from, to, insert: option.apply ?? option.label },
      selection: { anchor: result.from + (option.apply ?? option.label).length },
    });
  const doc = v.state.doc.toString();
  const head = v.state.selection.main.head;
  return `${doc.slice(0, head)}@${doc.slice(head)}`;
}

describe("link completion", () => {
  it("lists the files in path order right after [[", async () => {
    const r = await complete(linkCompletion(catalog()), "See [[@");
    expect(r?.from).toBe(6);
    expect(labels(r)).toEqual([
      "Orchard/North Block/Soil",
      "Orchard/Plan",
      "Plan",
      "Recipes/Apple Butter",
      "media/soil-map.png",
    ]);
    expect(r?.filter).toBe(false);
  });

  it("ranks files and aliases as the quick switcher does", async () => {
    const soil = await complete(linkCompletion(catalog()), "[[soil@");
    expect(labels(soil)).toEqual(["Orchard/North Block/Soil", "media/soil-map.png"]);
    const dirt = await complete(linkCompletion(catalog()), "x [[dirt@");
    expect(labels(dirt)).toEqual(["Dirt"]);
    expect(dirt?.options[0]?.detail).toBe("Orchard/North Block/Soil");
  });

  it("writes the file's link text and closes the link", async () => {
    expect(accept(await complete(linkCompletion(catalog()), "a [[pla@ b"), "Orchard/Plan")).toBe(
      "a [[Orchard/Plan]]@ b",
    );
    expect(accept(await complete(linkCompletion(catalog()), "[[pla@]] b"), "Plan")).toBe("[[Plan]]@ b");
    expect(accept(await complete(linkCompletion(catalog()), "[[map@"), "media/soil-map.png")).toBe(
      "[[media/soil-map.png]]@",
    );
  });

  it("writes an alias after the link text", async () => {
    expect(accept(await complete(linkCompletion(catalog()), "[[road@"), "Roadmap")).toBe("[[Plan|Roadmap]]@");
  });

  it("lists a note's headings after #, and the edited note's after [[#", async () => {
    const all = await complete(linkCompletion(catalog()), "[[Plan#@");
    expect(all?.from).toBe(7);
    expect(labels(all)).toEqual(["Plan", "Goals", "Tasks"]);
    expect(labels(await complete(linkCompletion(catalog()), "[[Plan#go@"))).toEqual(["Goals"]);
    expect(accept(await complete(linkCompletion(catalog()), "[[Plan#go@"), "Goals")).toBe("[[Plan#Goals]]@");
    const c = catalog();
    expect(labels(await complete(linkCompletion(c), "[[#ta@"))).toEqual(["Tasks"]);
    expect(c.asked).toEqual(["headings:"]);
  });

  it("lists a note's block IDs after #^", async () => {
    const c = catalog();
    const all = await complete(linkCompletion(c), "[[Soil#^@");
    expect(all?.from).toBe(8);
    expect(labels(all)).toEqual(["ph-test", "drainage"]);
    expect(all?.options[1]?.detail).toBe("Drains well ^drainage");
    expect(c.asked).toEqual(["blocks:Soil"]);
    expect(accept(await complete(linkCompletion(catalog()), "[[Soil#^dr@"), "drainage")).toBe("[[Soil#^drainage]]@");
  });

  it("lists nothing for an unknown note's headings", async () => {
    expect(labels(await complete(linkCompletion(catalog()), "[[Nowhere#@"))).toEqual([]);
  });

  it("lists at most COMPLETION_LIMIT files", async () => {
    const many = Array.from({ length: 60 }, (_, i) => ({ path: `n${String(i).padStart(2, "0")}.md`, aliases: [] }));
    expect(COMPLETION_LIMIT).toBe(50);
    expect(labels(await complete(linkCompletion(catalog(many)), "[[@"))).toHaveLength(50);
    expect(labels(await complete(linkCompletion(catalog(many)), "[[n@"))).toHaveLength(50);
  });

  it("completes nothing outside an open link's target, or in code", async () => {
    for (const text of ["[[Plan|al@", "[[Plan]] @", "[[a\nb@", "text@", "[x@", "`[[pl@`", "```\n[[pl@\n```\n"]) {
      expect(await complete(linkCompletion(catalog()), text)).toBeNull();
    }
  });
});

describe("tag completion", () => {
  it("lists tags after a tag's #, best first, then by count and name", async () => {
    const r = await complete(tagCompletion(catalog()), "Text #ha@");
    expect(r?.from).toBe(6);
    expect(labels(r)).toEqual(["hardy", "harvest", "orchard/north"]);
    expect(r?.options[0]?.detail).toBe("5");
    expect(labels(await complete(tagCompletion(catalog()), "#or@"))).toEqual(["orchard/north"]);
    expect(labels(await complete(tagCompletion(catalog()), "- [ ] x #orchard/n@"))).toEqual(["orchard/north"]);
  });

  it("writes the tag", async () => {
    expect(accept(await complete(tagCompletion(catalog()), "a #harv@ b"), "harvest")).toBe("a #harvest@ b");
  });

  it("needs a tag's start and a letter typed", async () => {
    for (const text of ["#@", "a#ha@", "## He@", "x # h@", "`#ha@`", "```\n#ha@\n```\n"]) {
      expect(await complete(tagCompletion(catalog()), text)).toBeNull();
    }
  });
});
