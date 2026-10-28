// The join flow's container (docs/specs/ui.md#vault-manager,
// docs/specs/ui.md#settings): the form, the code to approve, and the vault
// it makes and joins, over data/sync.ts. Cancel stops it between the calls
// that notesd answers.
import { useEffect, useRef, useState } from "react";

import { openVault, useClient } from "../data/session";
import { byName, useNotes } from "../data/stores";
import { ensurePaired, hubURL, isCancelled, joinPaired, pairAgain, syncError } from "../data/sync";
import { refusal } from "../data/vaults";
import { loadSync } from "../data/vaultState";
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
  /** The person went back or cancelled. */
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

  // A panel that goes away stops the flow between the calls notesd answers.
  useEffect(() => {
    return () => controller.current?.abort();
  }, []);

  /** make registers the new vault at <location>/<hub vault name>; null when notesd refuses it. */
  async function make(name: string, live: () => boolean): Promise<number | null> {
    const path = vaultFolder(location ?? "", name);
    try {
      const added = await client.vault.add({ path, create: true });
      if (!live()) {
        // The person cancelled while notesd made it: forget it; its folder stays.
        await client.vault.remove({ id: added.id }).catch(() => undefined);
        return null;
      }
      useNotes.setState((s) => ({ vaults: [...s.vaults.filter((v) => v.id !== added.id), added].sort(byName) }));
      return added.id;
    } catch (err) {
      if (!live()) return null;
      setPhase("form");
      setError(refusal(err, path));
      return null;
    }
  }

  async function run(fields: JoinFields, signal: AbortSignal, id: number): Promise<void> {
    const live = () => generation.current === id && !signal.aborted;
    const fail = (err: unknown): void => {
      if (!live() || isCancelled(err)) return;
      setPhase("form");
      setError(syncError(err));
    };
    const onCode = (c: string): void => {
      if (!live()) return;
      setCode(c);
      setPhase("approve");
    };
    const url = hubURL(fields.url);
    const device = fields.device === "" ? undefined : fields.device;
    const created = vaultId === undefined;
    try {
      if (mode === "pair") {
        await pairAgain(client, { url, device }, onCode, signal);
        if (!live()) return;
        if (vaultId !== undefined) await loadSync(client, vaultId);
        onDone(vaultId ?? null);
        return;
      }
      // A device paired with the hub joins at once; another shows the code to approve first.
      const paired = await ensurePaired(client, url, device, onCode, signal);
      if (!live()) return;
      setPhase("joining");
      const made = created ? await make(fields.name, live) : vaultId;
      if (made === null || made === undefined) return;
      try {
        await joinPaired(client, { vaultId: made, url, name: fields.name }, paired, device, onCode, signal);
      } catch (err) {
        if (created) {
          // The new vault joins nothing: forget it, as notesd does; its folder stays.
          await client.vault.remove({ id: made }).catch(() => undefined);
          useNotes.setState((s) => ({ vaults: s.vaults.filter((v) => v.id !== made) }));
        }
        fail(err);
        return;
      }
      if (!live()) return;
      await loadSync(client, made);
      if (created) await openVault(client, made);
      onDone(made);
    } catch (err) {
      fail(err);
    }
  }

  function start(fields: JoinFields): void {
    controller.current?.abort();
    const next = new AbortController();
    controller.current = next;
    const id = generation.current + 1;
    generation.current = id;
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
