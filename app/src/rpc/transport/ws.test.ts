import { afterEach, describe, expect, it, vi } from "vitest";

import { Client } from "../gen/api";
import { connectWS } from "./ws";

// FakeSocket stands in for the browser's WebSocket: tests open and close it
// and push frames.
class FakeSocket {
  static last: FakeSocket | undefined;
  readonly sent: string[] = [];
  onopen: (() => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;

  constructor(readonly url: string) {
    FakeSocket.last = this;
  }

  send(data: string) {
    this.sent.push(data);
  }
}

function socket(): FakeSocket {
  if (!FakeSocket.last) throw new Error("no socket");
  return FakeSocket.last;
}

afterEach(() => {
  vi.unstubAllGlobals();
  FakeSocket.last = undefined;
});

describe("connectWS", () => {
  it("sends one line per frame and splits received frames into lines", async () => {
    vi.stubGlobal("WebSocket", FakeSocket);
    const connecting = connectWS("ws://127.0.0.1:7890/?token=t");
    socket().onopen?.();
    const client = new Client(await connecting);
    expect(socket().url).toBe("ws://127.0.0.1:7890/?token=t");

    const list = client.vault.list();
    const hello = client.rpc.hello({ protocol: 1, client: "test" });
    await Promise.resolve();
    expect(socket().sent).toHaveLength(2);
    expect(socket().sent.every((f) => f.endsWith("}\n") && f.indexOf("\n") === f.length - 1)).toBe(true);

    // Two replies in one frame, the second split across two.
    socket().onmessage?.({
      data: `{"jsonrpc":"2.0","id":1,"result":[]}\n{"jsonrpc":"2.0","id":2,"result":{"protocol":1,`,
    });
    socket().onmessage?.({ data: `"server":"notesd"}}\n` });
    await expect(list).resolves.toEqual([]);
    await expect(hello).resolves.toEqual({ protocol: 1, server: "notesd" });
  });

  it("rejects when the gateway refuses the connection", async () => {
    vi.stubGlobal("WebSocket", FakeSocket);
    const connecting = connectWS("ws://127.0.0.1:7890/?token=wrong");
    socket().onclose?.({ code: 1006 });
    await expect(connecting).rejects.toThrow("cannot reach notesd's dev gateway (1006)");
  });

  it("closes the session when the socket closes", async () => {
    vi.stubGlobal("WebSocket", FakeSocket);
    const connecting = connectWS("ws://127.0.0.1:7890/?token=t");
    socket().onopen?.();
    const transport = await connecting;
    let reason = "";
    transport.onClose((r) => (reason = r));
    socket().onclose?.({ code: 1001 });
    expect(reason).toBe("dev gateway closed (1001)");
  });
});
