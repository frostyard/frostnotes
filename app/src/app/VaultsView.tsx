// The vault manager's container (ADR-0024; docs/specs/ui.md#vault-manager):
// the whole window while there is no vault, else a dialog from Manage
// vaults…. It opens a vault in place, opens a folder or creates one with
// the system's folder picker (or a typed path in a browser), joins a vault
// on a hub where notesd syncs, and removes one from the list after asking.
import { useEffect, useState } from "react";

import { closeVaultManager } from "../data/overlays";
import { openVault, useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { addVault, removeVault } from "../data/vaults";
import { CreateVault } from "../features/vaults/CreateVault";
import { VaultManager } from "../features/vaults/VaultManager";
import { vaultFolder } from "../lib/vaultName";
import { ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { JoinHub } from "./JoinHub";
import { canPickFolders, pickFolder } from "./system";

/** VaultsView draws the vault manager over the window's vaults. */
export function VaultsView(props: { mode: "window" | "dialog" }) {
  const client = useClient();
  const vaults = useNotes((s) => s.vaults);
  const current = useNotes((s) => s.vaultId);
  const [creating, setCreating] = useState(false);
  const [joining, setJoining] = useState(false);
  const [location, setLocation] = useState<string | null>(null);
  const [syncAvailable, setSyncAvailable] = useState(true);
  const canPick = canPickFolders();
  // vault.add and vault.remove admit local callers only (lodge-app.md).
  const manage = useNotes((s) => s.hello?.caller !== "session");

  // Join needs sync, which admits local callers only: a notesd without it
  // answers unavailable, or methodNotFound. Any other error keeps Join.
  useEffect(() => {
    let live = true;
    const probe = async () => {
      try {
        await client.sync.hubs();
      } catch (err) {
        const missing =
          err instanceof RPCError && (err.code === ErrorCode.unavailable || err.code === ErrorCode.methodNotFound);
        if (live && missing) setSyncAvailable(false);
      }
    };
    void probe();
    return () => {
      live = false;
    };
  }, [client]);

  const close = () => {
    setCreating(false);
    setJoining(false);
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
  const join = <JoinHub onDone={close} onCancel={() => setJoining(false)} />;
  return (
    <VaultManager
      vaults={vaults}
      current={current}
      mode={props.mode}
      canPick={canPick}
      manage={manage}
      syncAvailable={syncAvailable}
      panel={manage ? (creating ? form : joining ? join : undefined) : undefined}
      onOpen={(id) => {
        if (id !== current) void openVault(client, id);
        close();
      }}
      onRemove={(entry) => {
        const vault = vaults.find((v) => v.id === entry.id);
        if (vault) void removeVault(client, vault);
      }}
      onCreate={() => {
        setJoining(false);
        setCreating(true);
      }}
      onJoin={() => {
        setCreating(false);
        setJoining(true);
      }}
      onOpenFolder={(path) =>
        void (path !== undefined ? Promise.resolve(path) : pickFolder("Open folder as vault")).then((p) => {
          if (p !== null) void addVault(client, p).then(added);
        })
      }
      onClose={close}
    />
  );
}
