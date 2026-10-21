// The production transport: lines travel over the Tauri bridge
// (src-tauri/src/bridge.rs) to notesd's Unix socket.
import { Channel, convertFileSrc, invoke } from "@tauri-apps/api/core";

import { asTransport, JsonRpcSession, type Transport } from "../transport";

type BridgeMessage = { kind: "line"; line: string } | { kind: "closed"; error: string };

export async function connectTauri(): Promise<Transport> {
  // notesblob://localhost/<hash>.<ext>, as this platform spells a custom
  // scheme (src-tauri/src/protocol.rs; ADR-0023).
  const session = new JsonRpcSession((line) => invoke("notesd_send", { line }), {
    blobURL: (hash, ext) => convertFileSrc(`${hash}.${ext}`, "notesblob"),
  });
  const channel = new Channel<BridgeMessage>();
  channel.onmessage = (m) => {
    if (m.kind === "line") session.receive(m.line);
    else session.close(m.error);
  };
  await invoke("notesd_connect", { onMessage: channel });
  return asTransport(session);
}
