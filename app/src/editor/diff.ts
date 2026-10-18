// The difference between two texts as one change (ADR-0022): a merged
// answer or another writer's content applies as the change from the text
// the editor had, so the cursor, the selection and undo map through it.
import { diff } from "@codemirror/merge";
import { ChangeSet } from "@codemirror/state";

/**
 * DIFF_BUDGET bounds a diff, in milliseconds: past it, @codemirror/merge
 * falls back to a coarser diff, which still turns one text into the other.
 */
export const DIFF_BUDGET = 100;

/** changesBetween is the change that turns text a into text b. */
export function changesBetween(a: string, b: string): ChangeSet {
  if (a === b) return ChangeSet.empty(a.length);
  const parts = diff(a, b, { timeout: DIFF_BUDGET, scanLimit: 5000 });
  return ChangeSet.of(
    parts.map((c) => ({ from: c.fromA, to: c.toA, insert: b.slice(c.fromB, c.toB) })),
    a.length,
  );
}
