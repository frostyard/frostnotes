// The Outgoing links pane's list (docs/specs/ui.md, Right sidebar panes).
// Task T-0054 replaces this stub.
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
export function outgoingLinks(_links: readonly Link[], _self: string): OutgoingLink[] {
  return [];
}
