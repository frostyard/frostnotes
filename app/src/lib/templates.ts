// The template chooser's results (docs/specs/ui.md, Prompts; Insert
// template). Task T-0079 replaces these stubs.
import type { FuzzyMatch } from "./fuzzy";

/** TemplateResult is a row of the chooser: a template's path, its name, and the match in the name. */
export interface TemplateResult {
  path: string;
  name: string;
  /** Spans in the name; null with no query. */
  match: FuzzyMatch | null;
}

/** templateName is a template's name: its file name without ".md". */
export function templateName(path: string): string {
  return path;
}

/** templateResults lists the rows for query. */
export function templateResults(_paths: readonly string[], _query: string): TemplateResult[] {
  return [];
}
