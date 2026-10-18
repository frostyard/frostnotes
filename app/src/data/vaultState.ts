// The window's vault beyond its files (docs/design/app.md#data): its
// settings (vault.settings, again on vault.configured), its sync state
// (sync.status, then sync.changed) and its index's scan (vault.status,
// then vault.scan).
import { create } from "zustand";

import {
  type Client,
  ErrorCode,
  type Event,
  type ScanStatus,
  type SyncStatus,
  type VaultSettings,
} from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

/** VaultState is what the window knows of its vault besides its files. */
export interface VaultState {
  vaultId: number | null;
  /** The vault's settings; null until they load or when they cannot. */
  settings: VaultSettings | null;
  /** Why the settings cannot load: a vault.yaml that does not parse. */
  settingsError?: string;
  /** The vault's sync state; null when it syncs with no hub. */
  sync: SyncStatus | null;
  /** This notesd syncs (sync.status answers). */
  syncAvailable: boolean;
  /** The state of the vault's index. */
  scan: ScanStatus | null;
}

/** emptyVault is the state of vault before anything loads. */
export function emptyVault(vaultId: number | null): VaultState {
  return { vaultId, settings: null, sync: null, syncAvailable: false, scan: null };
}

export const useVaultState = create<VaultState>(() => emptyVault(null));

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

// current runs fn on the store only while it still shows vaultId.
function current(vaultId: number, fn: (s: VaultState) => Partial<VaultState>): void {
  useVaultState.setState((s) => (s.vaultId === vaultId ? fn(s) : s));
}

/** loadSettings reads the vault's settings. */
export async function loadSettings(c: Client, vaultId: number): Promise<void> {
  try {
    const settings = await c.vault.settings({ id: vaultId });
    current(vaultId, () => ({ settings, settingsError: undefined }));
  } catch (err) {
    current(vaultId, () => ({ settings: null, settingsError: message(err) }));
  }
}

/** loadSync reads the vault's sync state; a notesd that does not sync answers unavailable. */
export async function loadSync(c: Client, vaultId: number): Promise<void> {
  try {
    const all = await c.sync.status({ vaultId });
    current(vaultId, () => ({ sync: all.find((x) => x.vaultId === vaultId) ?? null, syncAvailable: true }));
  } catch (err) {
    const unavailable = err instanceof RPCError && err.code === ErrorCode.unavailable;
    current(vaultId, () => ({ sync: null, syncAvailable: !unavailable }));
  }
}

/** loadScan reads the state of the vault's index. */
export async function loadScan(c: Client, vaultId: number): Promise<void> {
  try {
    const scan = await c.vault.status({ id: vaultId });
    current(vaultId, () => ({ scan }));
  } catch {
    // The status bar shows nothing of the index.
  }
}

/** vaultEvent applies sync.changed and vault.scan, and reloads the settings on vault.configured. */
export function vaultEvent(c: Client, e: Event): void {
  const { vaultId } = useVaultState.getState();
  if (vaultId === null) return;
  switch (e.event) {
    case "vault.configured":
      if (e.data.id === vaultId) void loadSettings(c, vaultId);
      break;
    case "sync.changed":
      if (e.data.status.vaultId === vaultId) current(vaultId, () => ({ sync: e.data.status, syncAvailable: true }));
      break;
    case "vault.scan":
      if (e.data.status.vaultId === vaultId) current(vaultId, () => ({ scan: e.data.status }));
      break;
  }
}
