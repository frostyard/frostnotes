// The development browser's start (docs/design/app.md): VITE_NOTESD_WS
// connects to notesd's dev gateway (notesd -devgw) and VITE_MOCK=1 serves the
// UI from MockTransport. The web build inside lodge starts from
// lodge/main.tsx instead.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";

const gateway = import.meta.env.VITE_NOTESD_WS as string | undefined;
const connect: Connect = gateway
  ? async () => (await import("./rpc/transport/ws")).connectWS(gateway)
  : async () => {
      const { MockTransport, mockFiles, mockVaults } = await import("./rpc/mock/mock");
      return new MockTransport(mockVaults(), { latency: 15, files: mockFiles() });
    };

/** runDesktop renders the app into root. */
export function runDesktop(root: HTMLElement): void {
  createRoot(root).render(
    <StrictMode>
      <App connect={connect} />
    </StrictMode>,
  );
}
