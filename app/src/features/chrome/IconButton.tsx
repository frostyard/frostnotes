// A chrome button (docs/specs/ui.md#layout): an icon, an accessible name,
// and a tooltip naming it and its keys. 28 × 28 in the title bar row and
// the ribbon, 24 × 24 in toolbars and view headers.
import type { MouseEvent, ReactNode } from "react";

export interface IconButtonProps {
  label: string;
  /** The keys that run it, as formatHotkey writes them ("Ctrl + O"). */
  keys?: string;
  icon: ReactNode;
  size?: 28 | 24;
  pressed?: boolean;
  disabled?: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}

/** iconButtonClass is a chrome button's look at a size, pressed or selected. */
export function iconButtonClass(size: 28 | 24 = 28, selected = false): string {
  return `flex shrink-0 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40 disabled:hover:bg-transparent ${
    size === 24 ? "size-6" : "size-7"
  } pointer-coarse:size-11 pointer-coarse:[&_svg]:size-5 ${selected ? "bg-selection-sidebar" : ""}`;
}

/** IconButton is a chrome button. */
export function IconButton(props: IconButtonProps) {
  return (
    <button
      type="button"
      aria-pressed={props.pressed}
      aria-label={props.label}
      title={props.keys ? `${props.label} (${props.keys})` : props.label}
      disabled={props.disabled}
      onClick={props.onClick}
      className={iconButtonClass(props.size, props.pressed)}
    >
      {props.icon}
    </button>
  );
}
