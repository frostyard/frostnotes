// The app's stores (ADR-0011): what notesd told us, kept current by Session,
// and the window's own UI state.
import { create } from "zustand";

import type { Event, Hello, Vault } from "../rpc/gen/api";

/** Connection is the state of the link to notesd. */
export type Connection = { state: "connecting" } | { state: "ready" } | { state: "lost"; reason: string };

/** NotesState is notesd's data as the UI shows it. */
export interface NotesState {
  connection: Connection;
  hello: Hello | null;
  /** Every registered vault, ordered as vault.list orders them. */
  vaults: Vault[];
}

export const useNotes = create<NotesState>(() => ({
  connection: { state: "connecting" },
  hello: null,
  vaults: [],
}));

/** UIState is the window's own state. */
export interface UIState {
  sidebarVisible: boolean;
  toggleSidebar: () => void;
}

export const useUI = create<UIState>((set) => ({
  sidebarVisible: true,
  toggleSidebar: () => set((s) => ({ sidebarVisible: !s.sidebarVisible })),
}));

/** byName orders vaults as vault.list does: by name ignoring case, then id. */
export function byName(a: Vault, b: Vault): number {
  const x = a.name.toLowerCase();
  const y = b.name.toLowerCase();
  return x < y ? -1 : x > y ? 1 : a.id - b.id;
}

/**
 * applyVaultEvent returns vaults after a vault event, or vaults itself for
 * any other event. It is idempotent, so an event the last vault.list already
 * reflects changes nothing.
 */
export function applyVaultEvent(vaults: Vault[], e: Event): Vault[] {
  switch (e.event) {
    case "vault.added": {
      const v = e.data.vault;
      return [...vaults.filter((x) => x.id !== v.id), v].sort(byName);
    }
    case "vault.removed":
      return vaults.some((x) => x.id === e.data.id) ? vaults.filter((x) => x.id !== e.data.id) : vaults;
    default:
      return vaults;
  }
}
