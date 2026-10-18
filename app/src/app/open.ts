// Where a click opens what it names (docs/design/app.md#workspace),
// as Obsidian opens links: in the current tab, Ctrl in a new tab,
// Ctrl+Alt to the right, a middle click in a new tab.
import type { OpenWhere } from "../workspace/ops";

/** ClickInput is what of a click decides where it opens. */
export interface ClickInput {
  ctrlKey: boolean;
  altKey: boolean;
  button: number;
}

/** whereFor returns where a click opens its target. */
export function whereFor(e: ClickInput): OpenWhere {
  if (e.ctrlKey && e.altKey) return "right";
  if (e.ctrlKey || e.button === 1) return "tab";
  return "here";
}
