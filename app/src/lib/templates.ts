// The template chooser's results (docs/specs/ui.md, Prompts; Insert
// template): unique template paths, ranked by fuzzy matches in their names.
import { type FuzzyMatch, prepareFuzzySearch, sortByScore } from "./fuzzy";

/** TemplateResult is a row of the chooser: a template's path, its name, and the match in the name. */
export interface TemplateResult {
  path: string;
  name: string;
  /** Spans in the name; null with no query. */
  match: FuzzyMatch | null;
}

/** templateName is a template's name: its file name without ".md". */
export function templateName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1).replace(/\.md$/i, "");
}

/** templateResults lists the rows for query. */
export function templateResults(paths: readonly string[], query: string): TemplateResult[] {
  const templates = [...new Set(paths)]
    .map((path) => ({ path, name: templateName(path) }))
    .sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()) || a.path.localeCompare(b.path));
  const trimmed = query.trim();
  if (!trimmed) return templates.map((template) => ({ ...template, match: null }));

  const search = prepareFuzzySearch(trimmed);
  const results: { path: string; name: string; match: FuzzyMatch }[] = [];
  for (const template of templates) {
    const match = search(template.name);
    if (match) results.push({ ...template, match });
  }
  return sortByScore(results);
}
