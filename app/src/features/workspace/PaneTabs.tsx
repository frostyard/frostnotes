// A sidebar's pane tabs (docs/specs/ui.md#sidebars-and-panes): one button
// per pane in the title bar row over the sidebar, the selected one drawn
// selected.
import type { ReactNode } from "react";

import { iconButtonClass } from "../chrome/IconButton";

/** PaneTab is a pane as its tab shows it. */
export interface PaneTab {
  id: string;
  name: string;
  icon: ReactNode;
}

export interface PaneTabsProps {
  /** The tab list's accessible name ("Left sidebar"). */
  label: string;
  panes: PaneTab[];
  selected: string;
  onSelect: (id: string) => void;
}

/** PaneTabs draws a sidebar's pane tabs. */
export function PaneTabs(props: PaneTabsProps) {
  return (
    <div role="tablist" aria-label={props.label} data-tauri-drag-region className="flex h-11 items-center gap-1 px-2">
      {props.panes.map((p) => (
        <button
          key={p.id}
          type="button"
          role="tab"
          aria-selected={p.id === props.selected}
          aria-label={p.name}
          title={p.name}
          onClick={() => props.onSelect(p.id)}
          className={iconButtonClass(28, p.id === props.selected)}
        >
          {p.icon}
        </button>
      ))}
    </div>
  );
}
