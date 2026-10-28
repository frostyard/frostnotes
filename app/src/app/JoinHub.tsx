// The join flow's container (docs/specs/ui.md#vault-manager,
// docs/specs/ui.md#settings): the form, the code to approve, and the vault
// it makes and joins, over data/sync.ts. The panel stays while the flow
// runs: the window holds a first vault's manager open (data/session.tsx,
// holdVaultOpen). Cancel, Back and the panel's going away all stop the flow,
// which then forgets a vault it made and did not join.
import { useEffect, useRef, useState } from "react";

import { notify } from "../data/overlays";
import { holdVaultOpen, openVault, useClient } from "../data/session";
import { byName, useNotes } from "../data/stores";
import { ensurePaired, hubURL, isCancelled, joinPaired, pairAgain, syncError } from "../data/sync";
import { loadSync } from "../data/vaultState";
import { refusal } from "../data/vaults";
import {
  type JoinFields,
  JoinHubForm,
  type JoinLocation,
  type JoinMode,
  type JoinPhase,
} from "../features/sync/JoinHubForm";
import { vaultFolder } from "../lib/vaultName";
import { canPickFolders, pickFolder } from "./system";

export interface JoinHubProps {
  /** "join" joins a vault with a hub; "pair" pairs again with a new code (a vault in state revoked). */
  mode?: JoinMode;
  /** The hub URL the form starts with: a vault's hub, for pair again. */
  url?: string;
  /**
   * The vault to join. Without one, the panel makes a new vault at
   * <location>/<hub vault name>: the vault manager's use.
   */
  vaultId?: number;
  /** The flow ended: the vault is joined (or paired again), or none was made. */
  onDone: (vaultId: number | null) => void;
  /** The person went back, cancelled, or the panel went away. */
  onCancel: () => void;
}

/** JoinHub draws the join flow and runs it. */
export function JoinHub(props: JoinHubProps) {
  const { mode = "join", vaultId, onDone, onCancel } = props;
  const client = useClient();
  const [phase, setPhase] = useState<JoinPhase>("form");
  const [code, setCode] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [location, setLocation] = useState<string | null>(null);
  const controller = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const mounted = useRef(false);

  // The panel's going away is a stop, as Cancel and Back are.
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);

  async function run(fields: JoinFields, signal: AbortSignal, id: number): Promise<void> {
    const stopped = () => generation.current !== id || signal.aborted;
    /** fail says why the flow failed: in the panel, or in a notice once it is gone. */
    const fail = (words: string): void => {
      if (mounted.current) {
        setPhase("form");
        setError(words);
      } else {
        notify(words);
      }
    };
    const onCode = (c: string): void => {
      if (!mounted.current || stopped()) return;
      setCode(c);
      setPhase("approve");
    };
    /** forget removes the vault the flow made and did not join; it says why notesd would not, or null. */
    const forget = async (id: number): Promise<string | null> => {
      try {
        await client.vault.remove({ id });
      } catch (err) {
        return err instanceof Error ? err.message : String(err);
      }
      // The session's vault.removed may have dropped it from the list already.
      if (useNotes.getState().vaults.some((v) => v.id === id)) {
        useNotes.setState((s) => ({ vaults: s.vaults.filter((v) => v.id !== id) }));
      }
      return null;
    };
    const url = hubURL(fields.url);
    const device = fields.device === "" ? undefined : fields.device;
    const created = vaultId === undefined;
    let made: number | null = null;
    let joined = false;
    let reason = "";
    try {
      if (mode === "pair") {
        await pairAgain(client, { url, device }, onCode, signal);
        if (stopped()) return;
        if (vaultId !== undefined) await loadSync(client, vaultId);
        if (stopped()) return;
        if (mounted.current) onDone(vaultId ?? null);
        return;
      }
      // A device paired with the hub joins at once; another shows the code to approve first.
      const paired = await ensurePaired(client, url, device, onCode, signal);
      if (stopped()) return;
      // From here the panel shows "Joining…" with no Back and no Cancel: the
      // calls below make and join the vault, and a stop cleans up after them.
      if (mounted.current) setPhase("joining");
      let target = vaultId ?? null;
      if (created) {
        const path = vaultFolder(location ?? "", fields.name);
        try {
          const added = await client.vault.add({ path, create: true });
          made = added.id;
          target = added.id;
          if (mounted.current) {
            useNotes.setState((s) => ({ vaults: [...s.vaults.filter((v) => v.id !== added.id), added].sort(byName) }));
          }
        } catch (err) {
          if (!stopped()) reason = refusal(err, path);
          return;
        }
      }
      if (target === null || stopped()) return;
      await joinPaired(client, { vaultId: target, url, name: fields.name }, paired, device, onCode, signal);
      // The vault is joined: a stop now keeps it, and only skips what follows.
      joined = true;
      if (stopped()) return;
      await loadSync(client, target);
      if (stopped()) return;
      if (created) {
        // The manager goes as the window gets the vault.
        holdVaultOpen(false);
        if (mounted.current) await openVault(client, target);
      }
      if (mounted.current) onDone(target);
    } catch (err) {
      if (!stopped() && !isCancelled(err)) reason = syncError(err);
    } finally {
      if (made !== null && !joined) {
        const problem = await forget(made);
        if (problem !== null) {
          const removal = [
            `Frostnotes could not remove the new vault "${fields.name}" from the list: ${problem}.`,
            "Remove it in Manage vaults.",
          ].join(" ");
          reason = reason === "" ? removal : `${reason} ${removal}`;
        }
      }
      if (generation.current === id) holdVaultOpen(false);
      if (reason !== "") fail(reason);
    }
  }

  function start(fields: JoinFields): void {
    controller.current?.abort();
    const next = new AbortController();
    controller.current = next;
    const id = generation.current + 1;
    generation.current = id;
    // The manager stays while the flow makes a first vault.
    holdVaultOpen(true);
    setError(null);
    setCode(null);
    setPhase("joining");
    void run(fields, next.signal, id);
  }

  function cancel(): void {
    controller.current?.abort();
    controller.current = null;
    onCancel();
  }

  const withLocation = mode === "join" && vaultId === undefined;
  const folder: JoinLocation = {
    path: location,
    canPick: canPickFolders(),
    onBrowse: () => {
      void pickFolder("Choose where the new vault goes").then((p) => {
        if (p !== null) setLocation(p);
      });
    },
    onLocation: setLocation,
  };

  return (
    <JoinHubForm
      mode={mode}
      phase={phase}
      url={props.url}
      code={code}
      error={error}
      location={withLocation ? folder : undefined}
      onJoin={start}
      onBack={cancel}
      onCancel={cancel}
    />
  );
}
