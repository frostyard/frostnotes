// The connection to notesd and the stores it feeds (docs/design/app.md#data):
// connect, rpc.hello, events.subscribe from the last sequence number the
// window applied, then load the vaults and the window's vault; reconnect
// every second when the connection ends, loading again.
import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from "react";

import { Client, type Event, PROTOCOL } from "../rpc/gen/api";
import type { Transport } from "../rpc/transport";
import { feed } from "./events";
import { emptyFiles, loadFiles, noteEvent, useFiles } from "./files";
import { usePrefs } from "./prefs";
import { applyVaultEvent, useNotes } from "./stores";
import { emptyVault, loadScan, loadSettings, loadSync, useVaultState, vaultEvent } from "./vaultState";

const ClientContext = createContext<Client | null>(null);

/** useClient returns the connected client; it must be used under Session. */
export function useClient(): Client {
  const c = useContext(ClientContext);
  if (!c) throw new Error("useClient outside <Session>");
  return c;
}

/** Connect opens a transport to notesd. */
export type Connect = () => Promise<Transport>;

/** RETRY_MS is how long Session waits before reconnecting. */
export const RETRY_MS = 1000;

/** CLIENT_NAME is what rpc.hello tells notesd, for its logs and versions. */
export const CLIENT_NAME = "frostnotes-app";

/**
 * Session connects with connect, completes rpc.hello, subscribes to events,
 * loads the stores, and renders children with the client; fallback until
 * the first connection. When the connection ends it reconnects every
 * RETRY_MS; the children stay, with the closed client, whose calls fail
 * until the new one arrives, so open editors keep what was typed.
 */
export function Session(props: { connect: Connect; children: ReactNode; fallback: ReactNode }) {
  const { connect } = props;
  const [client, setClient] = useState<Client | null>(null);

  useEffect(() => {
    let stopped = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let cleanup: (() => void) | undefined;

    const lost = (reason: string) => {
      cleanup?.();
      cleanup = undefined;
      useNotes.setState({ connection: { state: "lost", reason } });
      retry = setTimeout(attempt, RETRY_MS);
    };

    const attempt = async () => {
      useNotes.setState({ connection: { state: "connecting" } });
      try {
        const transport = await connect();
        if (stopped) return;
        const c = new Client(transport);
        const hello = await c.rpc.hello({ protocol: PROTOCOL, client: CLIENT_NAME });
        // Events wait until the stores have loaded, then apply in order:
        // the stores skip what their loads reflect, and the feed sees all.
        let held: Event[] | null = [];
        let liveFrom = 0;
        cleanup = transport.onEvent((e) => {
          if (held) held.push(e);
          else route(c, e, liveFrom);
        });
        const since = feed.since();
        const sub = await c.events.subscribe(since === undefined ? {} : { sinceSeq: since });
        feed.started(sub, since);
        liveFrom = sub.seq;
        await load(c, liveFrom);
        if (stopped) return;
        for (const e of held) route(c, e, liveFrom);
        held = null;
        useNotes.setState({ hello, connection: { state: "ready" } });
        setClient(c);
        transport.onClose((reason) => {
          if (!stopped) lost(reason);
        });
      } catch (err) {
        if (!stopped) lost(err instanceof Error ? err.message : String(err));
      }
    };
    void attempt();
    return () => {
      stopped = true;
      clearTimeout(retry);
      cleanup?.();
    };
  }, [connect]);

  if (!client) return <>{props.fallback}</>;
  return <ClientContext.Provider value={client}>{props.children}</ClientContext.Provider>;
}

// load lists the vaults and opens the window's vault: the one it showed
// last while it exists, else the first.
async function load(c: Client, since: number): Promise<void> {
  const vaults = await c.vault.list();
  useNotes.setState({ vaults });
  const last = usePrefs.getState().lastVault;
  const pick = vaults.find((v) => v.path === last) ?? vaults[0];
  await openVault(c, pick?.id ?? null, since);
}

/**
 * openVault makes vault id the window's vault, remembers it, and loads its
 * files, settings, sync state and index state. since is the last sequence
 * number notesd had sent; the files store applies events after it.
 */
export async function openVault(c: Client, id: number | null, since = feed.since() ?? 0): Promise<void> {
  const vault = useNotes.getState().vaults.find((v) => v.id === id);
  useNotes.setState({ vaultId: vault?.id ?? null });
  useVaultState.setState(emptyVault(vault?.id ?? null));
  if (!vault) {
    useFiles.setState((s) => emptyFiles(null, 0, s.generation + 1));
    return;
  }
  usePrefs.getState().setPrefs({ lastVault: vault.path });
  await Promise.all([
    loadFiles(c, vault.id, since),
    loadSettings(c, vault.id),
    loadSync(c, vault.id),
    loadScan(c, vault.id),
  ]);
}

/** useOpenVault returns a function that makes a vault the window's. */
export function useOpenVault(): (id: number) => Promise<void> {
  const c = useClient();
  return useCallback((id: number) => openVault(c, id), [c]);
}

// route passes an event to the feed, then to the stores, unless it is a
// replay the stores' loads already reflect.
function route(c: Client, e: Event, liveFrom: number): void {
  feed.deliver(e);
  if (e.seq !== undefined && e.seq <= liveFrom) return;
  switch (e.event) {
    case "vault.added":
    case "vault.removed": {
      useNotes.setState((s) => ({ vaults: applyVaultEvent(s.vaults, e) }));
      const { vaults, vaultId } = useNotes.getState();
      if (vaultId === null || !vaults.some((v) => v.id === vaultId)) void openVault(c, vaults[0]?.id ?? null);
      break;
    }
    case "note.changed":
    case "note.moved":
    case "note.removed":
      noteEvent(e);
      break;
    case "vault.configured":
    case "sync.changed":
    case "vault.scan":
      vaultEvent(c, e);
      break;
  }
}
