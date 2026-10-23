// The lodge transport: notesd's API over lodge's /app/notes/api WebSocket,
// opened and kept by @frostyard/lodge-client/rpc's connectLodgeAPI
// (client/v0.3.2): it opens while the page is visible, pings every 30 s so
// lodge keeps the session, closes a socket whose ping goes unanswered for
// 10 s (a phone that sleeps drops connections without a close), and
// reconnects after a drop (docs/design/app.md#in-a-browser). Files load
// from lodge-content by their hash (ADR-0023), which the session carries
// as blobURL.
import { connectLodgeAPI as connect } from "@frostyard/lodge-client/rpc";

import type { Lodge } from "../../lodge/lodge";
import { asTransport, type Transport } from "../transport";

/** PING_TIMEOUT_MS is how long a ping may go unanswered before the socket is dropped. */
export const PING_TIMEOUT_MS = 10_000;

/** connectLodgeAPI is notesd's API through lodge's shell, with files by hash from blobs. */
export function connectLodgeAPI(lodge: Lodge, blobs: (hash: string, ext: string) => string): Promise<Transport> {
  return connect(lodge, { blobURL: blobs, pingTimeoutMs: PING_TIMEOUT_MS }).then((api) => {
    const transport = asTransport(api);
    // Session owns reconnecting, with a fresh connection and its hello and
    // subscribe; stop this one reconnecting too, so two sockets never live.
    transport.onClose(() => api.close());
    return transport;
  });
}
