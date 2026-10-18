// The dev transport: notesd's API over the dev gateway's WebSocket
// (internal/devgw, notesd -devgw), for running the UI in a browser against a
// real notesd. Each frame carries one newline-terminated line, the same lines
// the Unix socket carries (docs/specs/rpc-protocol.md, Transports).
import { JsonRpcSession, type Transport } from "../transport";

/** connectWS connects to notesd's dev gateway at url (ws://…?token=…). */
export function connectWS(url: string): Promise<Transport> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const session = new JsonRpcSession(async (line) => ws.send(`${line}\n`), gatewayBlobs(url));
    let buffer = "";
    let open = false;
    ws.onmessage = (e) => {
      buffer += String(e.data);
      for (let i = buffer.indexOf("\n"); i >= 0; i = buffer.indexOf("\n")) {
        const line = buffer.slice(0, i);
        buffer = buffer.slice(i + 1);
        if (line !== "") session.receive(line);
      }
    };
    ws.onopen = () => {
      open = true;
      resolve(session);
    };
    ws.onclose = (e) => {
      if (!open) reject(new Error(`cannot reach notesd's dev gateway (${e.code})`));
      session.close(`dev gateway closed (${e.code})`);
    };
  });
}

/**
 * gatewayBlobs serves blobs from the gateway's /blobs/ with its token, as
 * an <img> sends no header (ADR-0023): ws://host/?token=T gives
 * http://host/blobs/<hash>.<ext>?token=T.
 */
export function gatewayBlobs(url: string): (hash: string, ext: string) => string {
  const u = new URL(url);
  const origin = `${u.protocol === "wss:" ? "https:" : "http:"}//${u.host}`;
  const token = encodeURIComponent(u.searchParams.get("token") ?? "");
  return (hash, ext) => `${origin}/blobs/${hash}.${ext}?token=${token}`;
}
