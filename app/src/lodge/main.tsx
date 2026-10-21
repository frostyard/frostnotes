// The web build's start inside lodge (docs/design/app.md#in-a-browser):
// lodge-client's handshake first (ready, then the shell's init), then
// notes' own WebSocket to /app/notes/api through lodge, with files by hash
// from lodge-content. Outside a frame connectLodge sends the page to its
// canonical URL in the shell, and never resolves.
import { connectLodge } from "@frostyard/lodge-client";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "../app/App";
import type { Connect, Retry } from "../data/session";
import { useNotes } from "../data/stores";
import { contentMAC, fromBase64url } from "../lib/sha256";
import { connectLodgeAPI } from "../rpc/transport/lodge";
import { LodgeBridge } from "./Bridge";

/**
 * WEB_RETRY is how the web app reconnects: a phone drops connections when
 * it sleeps or changes networks, so after 1 s, doubling to 30 s, and at
 * once when the page is shown again or the browser is back online.
 */
export const WEB_RETRY: Retry = { firstMs: 1000, maxMs: 30_000 };

/**
 * contentBlobs gives a file's URL on lodge-content: blobs/<mac>/<hash>.<ext>,
 * the mac an HMAC of the hash under the content key rpc.hello gave this
 * session (docs/specs/lodge-app.md#contentblobs); "" until hello has.
 * MACs are kept, since the editor asks for the same files often.
 */
export function contentBlobs(url: (path: string) => string): (hash: string, ext: string) => string {
  let key = "";
  let bytes: Uint8Array = new Uint8Array();
  const macs = new Map<string, string>();
  return (hash, ext) => {
    const now = useNotes.getState().hello?.contentKey ?? "";
    if (now === "") return "";
    if (now !== key) {
      key = now;
      bytes = fromBase64url(now);
      macs.clear();
    }
    let mac = macs.get(hash);
    if (mac === undefined) {
      mac = contentMAC(bytes, hash);
      macs.set(hash, mac);
    }
    return url(`blobs/${mac}/${hash}.${ext}`);
  };
}

/** runInLodge connects to the shell and to notesd, then renders the app into root. */
export async function runInLodge(root: HTMLElement): Promise<void> {
  const lodge = await connectLodge({ app: "notes" });
  const blobs = contentBlobs((path) => lodge.contentURL(path));
  const connect: Connect = () => connectLodgeAPI(lodge, blobs);
  const wake = (fn: () => void) =>
    lodge.on("visible", ({ visible }) => {
      if (visible) fn();
    });
  createRoot(root).render(
    <StrictMode>
      <App connect={connect} retry={{ ...WEB_RETRY, wake }}>
        <LodgeBridge lodge={lodge} />
      </App>
    </StrictMode>,
  );
}
