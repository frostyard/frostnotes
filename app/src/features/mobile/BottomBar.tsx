// The bottom bar of a narrow window (docs/specs/ui.md#narrow-windows): 52
// high plus the screen's bottom safe area, six buttons spaced evenly.
import { ChevronLeft, ChevronRight, Menu, Plus, Search } from "lucide-react";

import { TouchButton } from "./TouchButton";

export interface BottomBarProps {
  canBack: boolean;
  canForward: boolean;
  /** How many tabs the group holds. */
  tabs: number;
  onBack: () => void;
  onForward: () => void;
  onSwitcher: () => void;
  onNewTab: () => void;
  onTabs: (x: number, y: number) => void;
  onMenu: (x: number, y: number) => void;
}

/** BottomBar draws a narrow window's bottom bar. */
export function BottomBar(props: BottomBarProps) {
  return (
    <nav
      aria-label="Bottom bar"
      className="flex shrink-0 items-center justify-around border-t border-separator bg-toolbar"
      style={{ minHeight: 52, paddingBottom: "env(safe-area-inset-bottom)" }}
    >
      <TouchButton
        label="Navigate back"
        icon={<ChevronLeft size={20} />}
        disabled={!props.canBack}
        onClick={props.onBack}
      />
      <TouchButton
        label="Navigate forward"
        icon={<ChevronRight size={20} />}
        disabled={!props.canForward}
        onClick={props.onForward}
      />
      <TouchButton label="Open quick switcher" icon={<Search size={20} />} onClick={props.onSwitcher} />
      <TouchButton label="New tab" icon={<Plus size={20} />} onClick={props.onNewTab} />
      <button
        type="button"
        aria-label={`Tabs (${props.tabs})`}
        title="Tabs"
        className="flex size-11 items-center justify-center rounded-md text-secondary active:bg-selection-inactive"
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          props.onTabs(r.left, r.top);
        }}
      >
        <span className="flex size-6 items-center justify-center rounded-md border-2 border-current text-xs font-semibold">
          {props.tabs}
        </span>
      </button>
      <TouchButton
        label="Menu"
        icon={<Menu size={20} />}
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          props.onMenu(r.left, r.top);
        }}
      />
    </nav>
  );
}
