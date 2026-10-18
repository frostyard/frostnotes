// The vault manager's container (ADR-0024; docs/specs/ui.md#vault-manager):
// the whole window while there is no vault, else a dialog from Manage
// vaults…. It opens a vault in place, opens a folder or creates one with
// the system's folder picker (or a typed path in a browser), and removes
// one from the list after asking.
import { useState } from "react";

import { closeVaultManager } from "../data/overlays";
import { openVault, useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { addVault, removeVault } from "../data/vaults";
import { CreateVault } from "../features/vaults/CreateVault";
import { VaultManager } from "../features/vaults/VaultManager";
import { vaultFolder } from "../lib/vaultName";
import { canPickFolders, pickFolder } from "./system";

/** VaultsView draws the vault manager over the window's vaults. */
export function VaultsView(props: { mode: "window" | "dialog" }) {
  const client = useClient();
  const vaults = useNotes((s) => s.vaults);
  const current = useNotes((s) => s.vaultId);
  const [creating, setCreating] = useState(false);
  const [location, setLocation] = useState<string | null>(null);
  const canPick = canPickFolders();
  const close = () => {
    setCreating(false);
    if (props.mode === "dialog") closeVaultManager();
  };
  const added = (ok: boolean) => {
    if (ok) close();
  };
  const form = (
    <CreateVault
      location={location}
      canPick={canPick}
      onBrowse={() =>
        void pickFolder("Choose where the new vault goes").then((p) => {
          if (p !== null) setLocation(p);
        })
      }
      onLocation={setLocation}
      onCreate={(name) => {
        if (location !== null) void addVault(client, vaultFolder(location, name), true).then(added);
      }}
      onBack={() => setCreating(false)}
    />
  );
  return (
    <VaultManager
      vaults={vaults}
      current={current}
      mode={props.mode}
      canPick={canPick}
      panel={creating ? form : undefined}
      onOpen={(id) => {
        if (id !== current) void openVault(client, id);
        close();
      }}
      onRemove={(entry) => {
        const vault = vaults.find((v) => v.id === entry.id);
        if (vault) void removeVault(client, vault);
      }}
      onCreate={() => setCreating(true)}
      onOpenFolder={(path) =>
        void (path !== undefined ? Promise.resolve(path) : pickFolder("Open folder as vault")).then((p) => {
          if (p !== null) void addVault(client, p).then(added);
        })
      }
      onClose={close}
    />
  );
}
