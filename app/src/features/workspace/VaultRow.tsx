// The vault row at the foot of the left sidebar (docs/specs/ui.md#sidebars-and-panes):
// the vault's name opens the vault menu, which lists the vaults by name
// with the current one checked, then Manage vaults…; Settings sits at the
// right.

import { IconButton, Menu } from "@frostyard/lodge-ui/react";
import { ChevronsUpDown, Settings } from "lucide-react";
import { useState } from "react";

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
  /** Manage vaults…: the vault manager. */
  onManage: () => void;
  onSettings: () => void;
}

/** VaultRow draws the vault row and its menu. */
export function VaultRow(props: VaultRowProps) {
  const [menuAt, setMenuAt] = useState<{ x: number; y: number } | null>(null);
  const current = props.vaults.find((v) => v.id === props.current);
  return (
    <div className="relative flex h-9 shrink-0 items-center gap-1 border-t border-separator px-2">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={menuAt !== null}
        title={current?.path}
        onClick={(e) => {
          // The menu opens above the row, which sits at the window's foot.
          const r = e.currentTarget.getBoundingClientRect();
          setMenuAt(menuAt ? null : { x: r.left, y: r.top - (props.vaults.length + 1) * 28 - 23 });
        }}
        className="text-sidebar-row flex min-w-0 flex-1 items-center gap-1 rounded-md px-1 py-1 font-semibold hover:bg-selection-inactive pointer-coarse:min-h-11"
      >
        <span className="truncate">{current?.name ?? "No vault"}</span>
        <ChevronsUpDown size={12} className="shrink-0 text-secondary" aria-hidden />
      </button>
      <IconButton
        size={24}
        label="Settings"
        keys="Ctrl + ,"
        icon={Settings}
        disabled={props.settingsDisabled}
        onClick={props.onSettings}
      />
      {menuAt && (
        <Menu
          label="Vaults"
          items={[
            ...props.vaults.map((v) => ({
              kind: "item" as const,
              id: String(v.id),
              label: v.name,
              checked: v.id === props.current,
            })),
            { kind: "separator" as const },
            { kind: "item" as const, id: "manage", label: "Manage vaults…" },
          ]}
          x={menuAt.x}
          y={menuAt.y}
          onSelect={(id) => (id === "manage" ? props.onManage() : props.onChoose(Number(id)))}
          onClose={() => setMenuAt(null)}
        />
      )}
    </div>
  );
}
