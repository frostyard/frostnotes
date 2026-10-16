// The production transport: lines travel over the Tauri bridge
// (src-tauri/src/bridge.rs) to notesd's Unix socket.
import { Channel, invoke } from "@tauri-apps/api/core";

import { JsonRpcSession, type Transport } from "../transport";

type BridgeMessage = { kind: "line"; line: string } | { kind: "closed"; error: string };

export async function connectTauri(): Promise<Transport> {
  const session = new JsonRpcSession((line) => invoke("notesd_send", { line }));
  const channel = new Channel<BridgeMessage>();
  channel.onmessage = (m) => {
    if (m.kind === "line") session.receive(m.line);
    else session.close(m.error);
  };
  await invoke("notesd_connect", { onMessage: channel });
  return session;
}
