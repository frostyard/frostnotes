// The Outgoing links pane's list (docs/specs/ui.md, Right sidebar panes).
// Groups distinct targets with resolved files first, omitting self-links.
import type { Link } from "../rpc/gen/api";

/** OutgoingLink is a row of the Outgoing links pane. */
export interface OutgoingLink {
  /** What the row shows: the file's name (".md" dropped) or the target as written. */
  label: string;
  /** The file it resolves to; null when unresolved. */
  path: string | null;
  /** The target as first written: what an unresolved link creates. */
  target: string;
}

/** outgoingLinks lists the targets of a note's links, leaving out links to the note itself (self). */
export function outgoingLinks(links: readonly Link[], self: string): OutgoingLink[] {
  const resolvedRows: OutgoingLink[] = [];
  const unresolvedRows: OutgoingLink[] = [];
  const resolvedPaths = new Set<string>();
  const unresolvedTargets = new Set<string>();

  for (const { target, resolved } of links) {
    if (target === "" || resolved === self) continue;

    if (resolved !== undefined) {
      if (resolvedPaths.has(resolved)) continue;
      resolvedPaths.add(resolved);
      const label = resolved.slice(resolved.lastIndexOf("/") + 1).replace(/\.md$/i, "");
      resolvedRows.push({ label, path: resolved, target });
    } else {
      const key = target.toLowerCase();
      if (unresolvedTargets.has(key)) continue;
      unresolvedTargets.add(key);
      unresolvedRows.push({ label: target, path: null, target });
    }
  }

  return [...resolvedRows, ...unresolvedRows];
}
