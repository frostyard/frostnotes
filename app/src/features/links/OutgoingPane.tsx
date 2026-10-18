// The Outgoing links pane (docs/specs/ui.md, Right sidebar panes) lists
// targets to open or create, with unresolved links dimmed.
import type { OutgoingLink } from "../../lib/outgoing";
import { type PaneClick, paneClick, paneEmpty, paneRow } from "../panes/common";

export interface OutgoingPaneProps {
  /** The rows; null when no note is open. */
  links: readonly OutgoingLink[] | null;
  /** Open the link's file, or create the unresolved one. */
  onOpen: (link: OutgoingLink, click: PaneClick) => void;
}

/** OutgoingPane draws the Outgoing links pane. */
export function OutgoingPane({ links, onOpen }: OutgoingPaneProps) {
  if (links === null) return <p className={paneEmpty}>No note is open.</p>;
  if (links.length === 0) return <p className={paneEmpty}>No links found.</p>;

  return (
    <ul aria-label="Outgoing links" className="py-1">
      {links.map((link) => (
        <li key={link.path ?? link.target}>
          <button
            type="button"
            className={`${paneRow} w-[calc(100%-16px)] px-2 text-left${link.path === null ? " text-tertiary" : ""}`}
            title={link.path ?? link.target}
            onClick={(e) => onOpen(link, paneClick(e))}
            onAuxClick={(e) => {
              if (e.button === 1) onOpen(link, paneClick(e));
            }}
          >
            <span className="truncate">{link.label}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}
