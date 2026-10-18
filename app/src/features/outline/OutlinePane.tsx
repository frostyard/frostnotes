// The Outline pane (docs/specs/ui.md, Right sidebar panes): the active
// note's nested headings, with folding controls and heading navigation.
import { ChevronRight } from "lucide-react";
import type { Heading, OutlineRow } from "../../lib/outline";
import { paneEmpty, paneRow } from "../panes/common";

export interface OutlinePaneProps {
  /** The rows (outlineRows); null when no note is open. */
  rows: readonly OutlineRow[] | null;
  /** The index in rows of the heading the cursor is under; -1 for none. */
  current: number;
  /** Fold or unfold the heading that starts at from. */
  onToggle: (from: number) => void;
  /** Take the editor to the heading. */
  onGo: (heading: Heading) => void;
}

/** OutlinePane draws the Outline pane. */
export function OutlinePane({ rows, current, onToggle, onGo }: OutlinePaneProps) {
  if (rows === null) return <p className={paneEmpty}>No note is open.</p>;
  if (rows.length === 0) return <p className={paneEmpty}>No headings</p>;

  return (
    <div role="tree" aria-label="Outline" className="py-1">
      {rows.map(({ heading, depth, hasChildren, expanded }, index) => (
        <div
          key={heading.from}
          role="treeitem"
          tabIndex={-1}
          aria-level={depth + 1}
          aria-expanded={hasChildren ? expanded : undefined}
          aria-selected={index === current}
          className={`${paneRow} ${index === current ? "bg-selection-sidebar" : ""}`}
          style={{ paddingLeft: 8 + 16 * depth }}
        >
          <span className="flex w-4 shrink-0 items-center justify-center">
            {hasChildren && (
              <button
                type="button"
                aria-label={`${expanded ? "Fold" : "Unfold"} ${heading.text}`}
                tabIndex={-1}
                onClick={() => onToggle(heading.from)}
              >
                <ChevronRight size={14} className={`text-secondary ${expanded ? "rotate-90" : ""}`} />
              </button>
            )}
          </span>
          <button
            type="button"
            title={heading.text}
            className="min-w-0 flex-1 truncate text-left"
            onClick={() => onGo(heading)}
          >
            {heading.text}
          </button>
        </div>
      ))}
    </div>
  );
}
