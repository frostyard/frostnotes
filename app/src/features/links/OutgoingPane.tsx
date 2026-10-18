// The Outgoing links pane (docs/specs/ui.md, Right sidebar panes). Task
// T-0054 replaces this stub.
import type { OutgoingLink } from "../../lib/outgoing";
import type { PaneClick } from "../panes/common";

export interface OutgoingPaneProps {
  /** The rows; null when no note is open. */
  links: readonly OutgoingLink[] | null;
  /** Open the link's file, or create the unresolved one. */
  onOpen: (link: OutgoingLink, click: PaneClick) => void;
}

/** OutgoingPane draws the Outgoing links pane. */
export function OutgoingPane(_props: OutgoingPaneProps) {
  return null;
}
