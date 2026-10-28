// The join flow's container (docs/specs/ui.md#vault-manager,
// docs/specs/ui.md#settings): the form, the code to approve, and the vault
// it makes and joins, over data/sync.ts. Cancel stops the pairing wait; once
// the flow makes or joins the vault it runs to its end, even when the panel
// goes away (the session opens a first vault as notesd adds it).
import { useEffect, useRef, useState } from "react";

import { notify } from "../data/overlays";
import { openVault, useClient } from "../data/session";
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
  /** The person went back or cancelled, before the flow committed to a vault. */
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
  // committed says the flow makes or joins the vault already: a panel that
  // goes away then no longer stops it.
  const committed = useRef(false);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (!committed.current) controller.current?.abort();
    };
  }, []);

  async function run(fields: JoinFields, signal: AbortSignal, id: number): Promise<void> {
    // stopped says the flow must not go on: the person cancelled, or a newer
    // run replaced it. A panel that goes away stops the pairing wait only;
    // from vault.add on, the flow runs to its end.
    const stopped = () => generation.current !== id || signal.aborted;
    /** fail says why the last attempt failed: in the panel, or in a notice once it is gone. */
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
    /** make registers the new vault at <location>/<name>; null when notesd refuses it. */
    const make = async (name: string): Promise<number | null> => {
      const path = vaultFolder(location ?? "", name);
      try {
        const added = await client.vault.add({ path, create: true });
        if (mounted.current) {
          useNotes.setState((s) => ({ vaults: [...s.vaults.filter((v) => v.id !== added.id), added].sort(byName) }));
        }
        return added.id;
      } catch (err) {
        if (!stopped()) fail(refusal(err, path));
        return null;
      }
    };
    /** forget removes the vault a failed join left; it says why notesd would not, or null. */
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
    try {
      if (mode === "pair") {
        await pairAgain(client, { url, device }, onCode, signal);
        if (stopped()) return;
        committed.current = true;
        if (vaultId !== undefined) await loadSync(client, vaultId);
        if (stopped()) return;
        if (mounted.current) onDone(vaultId ?? null);
        return;
      }
      // A device paired with the hub joins at once; another shows the code to approve first.
      const paired = await ensurePaired(client, url, device, onCode, signal);
      if (stopped()) return;
      // The pairing is done: the flow makes or joins the vault from here on, with no Cancel.
      committed.current = true;
      if (mounted.current) setPhase("joining");
      const made = created ? await make(fields.name) : vaultId;
      if (made === null || made === undefined) return;
      if (stopped()) return;
      try {
        await joinPaired(client, { vaultId: made, url, name: fields.name }, paired, device, onCode, signal);
      } catch (err) {
        if (stopped() || isCancelled(err)) return;
        let words = syncError(err);
        if (created) {
          // The new vault joins nothing: forget it, as notesd does; its folder stays.
          const problem = await forget(made);
          if (problem !== null) {
            words = [
              words,
              `Frostnotes could not remove the new vault "${fields.name}" from the list: ${problem}.`,
              "Remove it in Manage vaults.",
            ].join(" ");
          }
        }
        fail(words);
        return;
      }
      if (stopped()) return;
      await loadSync(client, made);
      if (stopped()) return;
      if (mounted.current) {
        // Without the panel the session opened the new vault already.
        if (created) await openVault(client, made);
        onDone(made);
      }
    } catch (err) {
      if (!stopped() && !isCancelled(err)) fail(syncError(err));
    }
  }

  function start(fields: JoinFields): void {
    controller.current?.abort();
    const next = new AbortController();
    controller.current = next;
    const id = generation.current + 1;
    generation.current = id;
    committed.current = false;
    setError(null);
    setCode(null);
    setPhase("joining");
    void run(fields, next.signal, id);
  }

  function cancel(): void {
    controller.current?.abort();
    controller.current = null;
    generation.current += 1;
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
