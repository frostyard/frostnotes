// The vault row at the foot of the left sidebar (docs/specs/ui.md#sidebars-and-panes):
// the vault's name opens the vault menu; Settings sits at the right. The
// menu here is the vault switcher's first form, until batch 2's menus.
import { Check, ChevronsUpDown, Settings } from "lucide-react";
import { useState } from "react";

import { IconButton } from "../chrome/IconButton";

/** VaultChoice is a vault the menu lists. */
export interface VaultChoice {
  id: number;
  name: string;
  path: string;
}

export interface VaultRowProps {
  vaults: VaultChoice[];
  current: number | null;
  settingsDisabled: boolean;
  onChoose: (id: number) => void;
  onSettings: () => void;
}

/** VaultRow draws the vault row and its menu. */
export function VaultRow(props: VaultRowProps) {
  const [open, setOpen] = useState(false);
  const current = props.vaults.find((v) => v.id === props.current);
  return (
    <div className="relative flex h-9 shrink-0 items-center gap-1 border-t border-separator px-2">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        title={current?.path}
        onClick={() => setOpen(!open)}
        className="text-sidebar-row flex min-w-0 flex-1 items-center gap-1 rounded-md px-1 py-1 font-semibold hover:bg-selection-inactive"
      >
        <span className="truncate">{current?.name ?? "No vault"}</span>
        <ChevronsUpDown size={12} className="shrink-0 text-secondary" aria-hidden />
      </button>
      <IconButton
        size={24}
        label="Settings"
        keys="Ctrl + ,"
        icon={<Settings size={16} />}
        disabled={props.settingsDisabled}
        onClick={props.onSettings}
      />
      {open && (
        <div
          role="menu"
          aria-label="Vaults"
          className="absolute bottom-full left-2 z-20 mb-1 min-w-[180px] rounded-lg border border-separator bg-window p-1 shadow-[var(--popover-shadow)]"
        >
          {props.vaults.map((v) => (
            <button
              key={v.id}
              type="button"
              role="menuitemradio"
              aria-checked={v.id === props.current}
              title={v.path}
              onClick={() => {
                setOpen(false);
                props.onChoose(v.id);
              }}
              className="text-sidebar-row flex h-7 w-full items-center gap-2 rounded px-2 text-left hover:bg-accent hover:text-accent-contrast"
            >
              <span className="w-4 shrink-0">{v.id === props.current && <Check size={14} aria-hidden />}</span>
              <span className="truncate">{v.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
