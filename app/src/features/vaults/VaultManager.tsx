// The vault manager (docs/specs/ui.md#vault-manager). Task T-0082
// replaces this stub.
import type { ReactNode } from "react";

/** VaultEntry is a vault the manager lists. */
export interface VaultEntry {
  id: number;
  name: string;
  path: string;
}

export interface VaultManagerProps {
  /** The vaults, in the order shown (by name). */
  vaults: VaultEntry[];
  /** The window's vault, if any. */
  current: number | null;
  /** "window" fills its parent, without Close; "dialog" floats over the window. */
  mode: "window" | "dialog";
  /** The system has a folder picker: Open opens it. Else a path is typed. */
  canPick: boolean;
  /** When given, shown in the actions' place: the create form. */
  panel?: ReactNode;
  onOpen: (id: number) => void;
  onRemove: (vault: VaultEntry) => void;
  onCreate: () => void;
  /** Open folder as vault: no path with a picker, else the typed path, trimmed. */
  onOpenFolder: (path?: string) => void;
  onClose: () => void;
}

/** VaultManager draws the vault manager. */
export function VaultManager(_props: VaultManagerProps) {
  return null;
}
