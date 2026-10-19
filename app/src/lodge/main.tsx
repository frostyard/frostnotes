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
import { connectApi } from "../rpc/transport/ws";
import { LodgeBridge } from "./Bridge";

/**
 * WEB_RETRY is how the web app reconnects: a phone drops connections when
 * it sleeps or changes networks, so after 1 s, doubling to 30 s, and at
 * once when the page is shown again or the browser is back online.
 */
export const WEB_RETRY: Retry = { firstMs: 1000, maxMs: 30_000 };

/** runInLodge connects to the shell and to notesd, then renders the app into root. */
export async function runInLodge(root: HTMLElement): Promise<void> {
  const lodge = await connectLodge({ app: "notes" });
  const blobs = (hash: string, ext: string) => lodge.contentURL(`blobs/${hash}.${ext}`);
  const connect: Connect = () => connectApi(lodge.apiURL(), blobs);
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
