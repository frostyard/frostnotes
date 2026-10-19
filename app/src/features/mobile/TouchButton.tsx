// A button of the narrow layout's chrome (docs/specs/ui.md#narrow-windows):
// 44 × 44, a 20-pixel icon, and its name.
import type { MouseEvent, ReactNode } from "react";

export interface TouchButtonProps {
  label: string;
  icon: ReactNode;
  disabled?: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}

/** TouchButton is a 44 × 44 chrome button. */
export function TouchButton(props: TouchButtonProps) {
  return (
    <button
      type="button"
      aria-label={props.label}
      title={props.label}
      disabled={props.disabled}
      onClick={props.onClick}
      className="flex size-11 shrink-0 items-center justify-center rounded-md text-secondary active:bg-selection-inactive disabled:opacity-40"
    >
      {props.icon}
    </button>
  );
}
