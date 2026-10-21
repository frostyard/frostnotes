// A button of the narrow layout's chrome (docs/specs/ui.md#narrow-windows):
// 44 × 44, a 20-pixel icon, and its name. lodge-ui's IconButton is the
// toolbar's: this one is the phone's, with its own size and its press state.
import type { LucideIcon } from "lucide-react";
import type { MouseEvent } from "react";

export interface TouchButtonProps {
  label: string;
  icon: LucideIcon;
  disabled?: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}

/** TouchButton is a 44 × 44 chrome button. */
export function TouchButton(props: TouchButtonProps) {
  const Icon = props.icon;
  return (
    <button
      type="button"
      aria-label={props.label}
      title={props.label}
      disabled={props.disabled}
      onClick={props.onClick}
      className="flex size-11 shrink-0 items-center justify-center rounded-md text-secondary active:bg-selection-inactive disabled:opacity-40"
    >
      <Icon size={20} aria-hidden="true" />
    </button>
  );
}
