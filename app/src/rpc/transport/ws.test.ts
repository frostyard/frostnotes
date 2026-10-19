import { afterEach, describe, expect, it, vi } from "vitest";

import { Client } from "../gen/api";
import { connectApi, connectWS, gatewayBlobs, PING_EVERY_MS, PING_TIMEOUT_MS } from "./ws";

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

describe("gatewayBlobs", () => {
  it("serves blobs from the gateway's /blobs/ with its token", () => {
    const url = gatewayBlobs("ws://127.0.0.1:7788/?token=a%2Bb");
    expect(url("ab12", "png")).toBe("http://127.0.0.1:7788/blobs/ab12.png?token=a%2Bb");
    expect(gatewayBlobs("wss://hub.example/ws?token=t")("cd", "mp4")).toBe("https://hub.example/blobs/cd.mp4?token=t");
  });
});

describe("connectApi", () => {
  // A page whose visibility a test sets, with its listeners.
  function page(state: DocumentVisibilityState = "visible") {
    const listeners = new Set<() => void>();
    return {
      visibilityState: state,
      addEventListener: (_: string, fn: () => void) => listeners.add(fn),
      removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
      show(next: DocumentVisibilityState) {
        this.visibilityState = next;
        for (const fn of listeners) fn();
      },
      listeners,
    };
  }
  const blobs = (hash: string, ext: string) => `https://lodge-content.example.ts.net/notes/blobs/${hash}.${ext}`;
  const pings = () => socket().sent.filter((f) => f.includes('"rpc.ping"'));

  afterEach(() => {
    vi.useRealTimers();
  });

  it("connects to lodge's apiURL, and loads files from lodge-content", async () => {
    const doc = page();
    const connecting = connectApi("wss://lodge.example.ts.net/app/notes/api", blobs, {
      WebSocket: FakeSocket as unknown as typeof WebSocket,
      document: doc as unknown as Document,
    });
    socket().onopen?.();
    const transport = await connecting;
    expect(socket().url).toBe("wss://lodge.example.ts.net/app/notes/api");
    expect(transport.blobURL?.("ab12", "png")).toBe("https://lodge-content.example.ts.net/notes/blobs/ab12.png");
  });

  it("pings every 30 s while the page is visible, at once when it is shown, and never while hidden", async () => {
    vi.useFakeTimers();
    const doc = page();
    const connecting = connectApi("wss://lodge.example.ts.net/app/notes/api", blobs, {
      WebSocket: FakeSocket as unknown as typeof WebSocket,
      document: doc as unknown as Document,
    });
    socket().onopen?.();
    await connecting;
    expect(pings()).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(PING_EVERY_MS);
    expect(pings()).toHaveLength(1);
    const id = JSON.parse(pings()[0] ?? "{}").id as number;
    socket().onmessage?.({ data: `{"jsonrpc":"2.0","id":${id},"result":null}\n` });
    await vi.advanceTimersByTimeAsync(0);
    doc.show("hidden");
    await vi.advanceTimersByTimeAsync(3 * PING_EVERY_MS);
    expect(pings()).toHaveLength(1);
    doc.show("visible");
    expect(pings()).toHaveLength(2);
  });

  it("closes a connection whose ping goes unanswered for 10 s", async () => {
    vi.useFakeTimers();
    const doc = page();
    const connecting = connectApi("wss://lodge.example.ts.net/app/notes/api", blobs, {
      WebSocket: FakeSocket as unknown as typeof WebSocket,
      document: doc as unknown as Document,
    });
    const s = socket();
    let closed: [number, string] | undefined;
    (s as unknown as { close: (code: number, reason: string) => void }).close = (code, reason) => {
      closed = [code, reason];
      s.onclose?.({ code });
    };
    s.onopen?.();
    const transport = await connecting;
    let reason = "";
    transport.onClose((r) => (reason = r));
    await vi.advanceTimersByTimeAsync(PING_EVERY_MS + PING_TIMEOUT_MS - 1);
    expect(closed).toBeUndefined();
    await vi.advanceTimersByTimeAsync(1);
    expect(closed).toEqual([4000, "notesd did not answer a ping"]);
    expect(reason).toBe("the connection through lodge closed (4000)");
    expect(doc.listeners.size).toBe(0);
  });

  it("rejects when lodge refuses the upgrade", async () => {
    const connecting = connectApi("wss://lodge.example.ts.net/app/notes/api", blobs, {
      WebSocket: FakeSocket as unknown as typeof WebSocket,
      document: page() as unknown as Document,
    });
    socket().onclose?.({ code: 1006 });
    await expect(connecting).rejects.toThrow("cannot reach notesd through lodge (1006)");
  });
});
