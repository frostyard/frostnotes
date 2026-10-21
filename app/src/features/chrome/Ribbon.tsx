// The ribbon (docs/specs/ui.md#ribbon): a column of buttons that run
// commands, under the title bar row's Toggle left sidebar.

import { IconButton } from "@frostyard/lodge-ui/react";
import type { LucideIcon } from "lucide-react";

/** RibbonItem is a ribbon button and the command it runs. */
export interface RibbonItem {
  command: string;
  label: string;
  icon: LucideIcon;
  keys?: string;
  /** The command does not apply now. */
  disabled: boolean;
}

export interface RibbonProps {
  items: RibbonItem[];
  onRun: (command: string) => void;
}

/** Ribbon draws the ribbon's buttons. */
export function Ribbon({ items, onRun }: RibbonProps) {
  return (
    <nav aria-label="Ribbon" className="flex flex-col items-center gap-1 pt-2">
      {items.map((item) => (
        <IconButton
          key={item.command}
          label={item.label}
          keys={item.keys}
          icon={item.icon}
          disabled={item.disabled}
          onClick={() => onRun(item.command)}
        />
      ))}
    </nav>
  );
}
