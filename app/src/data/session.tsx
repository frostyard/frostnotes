// The connection to notesd and the stores it feeds (ADR-0011): connect,
// rpc.hello, events.subscribe, then load; reconnect when the connection ends.
import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { Client, type Event, PROTOCOL } from "../rpc/gen/api";
import type { Transport } from "../rpc/transport";
import { applyVaultEvent, useNotes } from "./stores";

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

/** CLIENT_NAME is what rpc.hello tells notesd, for its logs. */
export const CLIENT_NAME = "frostnotes-app";

/**
 * Session connects with connect, completes rpc.hello, subscribes to events,
 * loads the vaults, and renders children with the client. When the
 * connection ends it reconnects every RETRY_MS, rendering fallback meanwhile.
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
      setClient(null);
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
        // Events that arrive before vault.list answers are held and applied
        // to its result (applyVaultEvent is idempotent), so none is lost.
        let held: Event[] | null = [];
        cleanup = transport.onEvent((e) => {
          if (held) held.push(e);
          else useNotes.setState((s) => ({ vaults: applyVaultEvent(s.vaults, e) }));
        });
        await c.events.subscribe({});
        let vaults = await c.vault.list();
        if (stopped) return;
        for (const e of held) vaults = applyVaultEvent(vaults, e);
        held = null;
        useNotes.setState({ hello, vaults, connection: { state: "ready" } });
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
