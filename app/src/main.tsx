import "./styles/app.css";

import { invoke, isTauri } from "@tauri-apps/api/core";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";

// VITE_NOTESD_WS connects to notesd's dev gateway (notesd -devgw) and
// VITE_MOCK=1 serves the UI from MockTransport, both in any browser;
// otherwise the Tauri bridge connects to notesd's socket.
const gateway = import.meta.env.VITE_NOTESD_WS as string | undefined;
const connect: Connect = gateway
  ? async () => (await import("./rpc/transport/ws")).connectWS(gateway)
  : import.meta.env.VITE_MOCK === "1"
    ? async () => {
        const { MockTransport, mockVaults } = await import("./rpc/mock/mock");
        return new MockTransport(mockVaults(), { latency: 15 });
      }
    : async () => (await import("./rpc/transport/tauri")).connectTauri();

/** SpikeRun says which M0 spike to show instead of the app, and how. */
interface SpikeRun {
  name: string;
  /** Run the measurements as soon as the page loads. */
  auto: boolean;
  /** Print the results through the shell and exit (FROSTNOTES_SPIKE). */
  report: boolean;
}

// A spike runs from ?spike=NAME (with &run to start measuring at once) or,
// in the desktop app, from FROSTNOTES_SPIKE=NAME, which measures, reports and
// exits, or NAME:manual, which waits for the person (src-tauri/src/spike.rs).
async function spikeRun(): Promise<SpikeRun | null> {
  const query = new URLSearchParams(window.location.search);
  const name = query.get("spike");
  if (name) return { name, auto: query.has("run"), report: false };
  if (!isTauri()) return null;
  const env = await invoke<string | null>("spike_mode");
  if (!env) return null;
  const manual = env.endsWith(":manual");
  return { name: manual ? env.slice(0, -":manual".length) : env, auto: !manual, report: !manual };
}

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
const spike = await spikeRun();
if (spike) {
  const { Spike } = await import("./spike/Spike");
  createRoot(root).render(<Spike name={spike.name} auto={spike.auto} report={spike.report} />);
} else {
  createRoot(root).render(
    <StrictMode>
      <App connect={connect} />
    </StrictMode>,
  );
}
