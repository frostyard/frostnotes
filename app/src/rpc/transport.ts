// The line-level contract between the generated Client (gen/api.ts) and a
// connection to notesd: the rpc subpath of @frostyard/lodge-client
// (client/v0.3.2), whose JsonRpcSession multiplexes JSON-RPC in the
// webview (docs/specs/rpc-protocol.md). The Tauri, gateway and lodge
// transports implement it. The session hands events over in the kit's
// envelope; the app types what they carry (gen/api.ts), and asTransport is
// the one place the two meet.
import type { Transport as RpcTransport } from "@frostyard/lodge-client/rpc";

import type { Event } from "./gen/api";

export type { EventEnvelope } from "@frostyard/lodge-client/rpc";
export { JsonRpcSession, RPCError } from "@frostyard/lodge-client/rpc";

/** Transport is what the app's client calls, with notesd's events typed. */
export interface Transport {
  call<T>(method: string, params: unknown): Promise<T>;
  /** Subscribe to server events; returns an unsubscribe function. */
  onEvent(handler: (event: Event) => void): () => void;
  /** Called once when the connection ends. */
  onClose(handler: (reason: string) => void): () => void;
  /**
   * The URL a vault file's bytes load from by its hash and extension
   * (ADR-0023); "" when the transport serves none.
   */
  blobURL?(hash: string, ext: string): string;
}

/** asTransport is the subpath's session with notesd's events (gen/api.ts). */
export function asTransport(session: RpcTransport): Transport {
  const transport: Transport = {
    call: <T>(method: string, params: unknown) => session.call<T>(method, params),
    onEvent: (handler) => session.onEvent((envelope) => handler(envelope as Event)),
    onClose: (handler) => session.onClose(handler),
  };
  const blobURL = session.blobURL;
  if (blobURL !== undefined) transport.blobURL = blobURL;
  return transport;
}
