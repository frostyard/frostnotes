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

/** PING_EVERY_MS is how often the web app pings notesd while its page is visible. */
export const PING_EVERY_MS = 30_000;
/** PING_TIMEOUT_MS is how long a ping may go unanswered before the connection is dropped. */
export const PING_TIMEOUT_MS = 10_000;

/** ApiOptions tune connectApi; tests give fakes. */
export interface ApiOptions {
  /** The WebSocket constructor; the browser's by default. */
  WebSocket?: typeof WebSocket;
  /** The document whose visibility gates the heartbeat; the page's by default. */
  document?: Pick<Document, "visibilityState" | "addEventListener" | "removeEventListener">;
}

/**
 * connectApi connects to notesd's /api through lodge (docs/design/app.md#in-a-browser):
 * url is lodge-client's apiURL(), with no token, since lodge signed the
 * browser in and names it to notesd; blobs is where files load by hash,
 * lodge-content's contentURL("blobs/<mac>/<hash>.<ext>") (contentBlobs in
 * lodge/main.tsx). While the page is
 * visible it calls rpc.ping every PING_EVERY_MS, at once when the page
 * becomes visible, and closes a connection whose ping goes unanswered for
 * PING_TIMEOUT_MS: lodge ends a session after 90 s without a frame, and a
 * sleeping phone drops connections without a close.
 */
export function connectApi(
  url: string,
  blobs: (hash: string, ext: string) => string,
  opts: ApiOptions = {},
): Promise<Transport> {
  const WS = opts.WebSocket ?? WebSocket;
  const doc = opts.document ?? document;
  return new Promise((resolve, reject) => {
    const ws = new WS(url);
    const session = new JsonRpcSession(async (line) => ws.send(`${line}\n`), blobs);
    let buffer = "";
    let open = false;
    let timer: ReturnType<typeof setInterval> | undefined;
    let waiting = false;
    const ping = () => {
      if (waiting || doc.visibilityState !== "visible") return;
      waiting = true;
      const deadline = setTimeout(() => ws.close(4000, "notesd did not answer a ping"), PING_TIMEOUT_MS);
      session.call("rpc.ping", {}).then(
        () => {
          clearTimeout(deadline);
          waiting = false;
        },
        () => clearTimeout(deadline),
      );
    };
    const onVisible = () => {
      if (doc.visibilityState === "visible") ping();
    };
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
      timer = setInterval(ping, PING_EVERY_MS);
      doc.addEventListener("visibilitychange", onVisible);
      resolve(session);
    };
    ws.onclose = (e) => {
      clearInterval(timer);
      doc.removeEventListener("visibilitychange", onVisible);
      if (!open) reject(new Error(`cannot reach notesd through lodge (${e.code})`));
      session.close(`the connection through lodge closed (${e.code})`);
    };
  });
}
