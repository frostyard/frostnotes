// The left sidebar (ADR-0011, Obsidian's layout): for now, the vaults notesd
// owns. The file tree, search, tags and bookmarks join it in M3.
import { Vault as VaultIcon } from "lucide-react";

import type { Vault } from "../../rpc/gen/api";

export interface VaultSidebarProps {
  width: number;
  vaults: Vault[];
}

export function VaultSidebar({ width, vaults }: VaultSidebarProps) {
  return (
    <nav
      aria-label="Vaults"
      className="flex shrink-0 flex-col overflow-y-auto border-r border-separator bg-sidebar py-2"
      style={{ width: `${width}px` }}
    >
      <h2 className="text-sidebar-section px-4 py-1 text-tertiary uppercase">Vaults</h2>
      {vaults.length === 0 ? (
        <p className="text-sidebar-row px-4 py-1 text-tertiary">None yet</p>
      ) : (
        <ul>
          {vaults.map((v) => (
            <li key={v.id} title={v.path} className="text-sidebar-row flex items-center gap-2 px-4 py-1">
              <VaultIcon size={14} className="shrink-0 text-secondary" aria-hidden />
              <span className="truncate">{v.name}</span>
            </li>
          ))}
        </ul>
      )}
    </nav>
  );
}
