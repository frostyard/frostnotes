import { describe, expect, it } from "vitest";

import { Client, ErrorCode, PROTOCOL } from "./gen/api";
import { JsonRpcSession, RPCError } from "./transport";

function fakeServer() {
  const sent: Record<string, unknown>[] = [];
  const session = new JsonRpcSession(async (line) => {
    sent.push(JSON.parse(line) as Record<string, unknown>);
  });
  return { session, sent };
}

describe("JsonRpcSession", () => {
  it("matches responses to calls by id, in any order", async () => {
    const { session, sent } = fakeServer();
    const client = new Client(session);
    const hello = client.rpc.hello({ protocol: PROTOCOL, client: "test" });
    const list = client.vault.list();
    await Promise.resolve();
    expect(sent.map((m) => m.method)).toEqual(["rpc.hello", "vault.list"]);
    expect(sent[1]?.params).toEqual({});
    session.receive(JSON.stringify({ jsonrpc: "2.0", id: sent[1]?.id, result: [] }));
    session.receive(JSON.stringify({ jsonrpc: "2.0", id: sent[0]?.id, result: { protocol: 1, server: "notesd" } }));
    await expect(list).resolves.toEqual([]);
    await expect(hello).resolves.toEqual({ protocol: 1, server: "notesd" });
  });

  it("rejects with RPCError carrying the code", async () => {
    const { session, sent } = fakeServer();
    const remove = new Client(session).vault.remove({ id: 9 });
    await Promise.resolve();
    session.receive(
      JSON.stringify({
        jsonrpc: "2.0",
        id: sent[0]?.id,
        error: { code: ErrorCode.notFound, message: "vault 9: not found" },
      }),
    );
    await expect(remove).rejects.toEqual(new RPCError(ErrorCode.notFound, "vault 9: not found"));
  });

  it("resolves a method without a result to undefined", async () => {
    const { session, sent } = fakeServer();
    const remove = new Client(session).vault.remove({ id: 1 });
    await Promise.resolve();
    session.receive(JSON.stringify({ jsonrpc: "2.0", id: sent[0]?.id, result: null }));
    await expect(remove).resolves.toBeUndefined();
  });

  it("delivers events and fails pending calls on close", async () => {
    const { session } = fakeServer();
    const events: unknown[] = [];
    session.onEvent((e) => events.push(e));
    session.receive(
      JSON.stringify({ jsonrpc: "2.0", method: "event", params: { seq: 3, event: "vault.removed", data: { id: 1 } } }),
    );
    expect(events).toEqual([{ seq: 3, event: "vault.removed", data: { id: 1 } }]);
    const pending = new Client(session).vault.list();
    await Promise.resolve();
    let reason = "";
    session.onClose((r) => (reason = r));
    session.close("notesd closed the connection");
    await expect(pending).rejects.toThrow("notesd closed the connection");
    expect(reason).toBe("notesd closed the connection");
    await expect(new Client(session).vault.list()).rejects.toThrow("notesd connection closed");
  });

  it("closes on a malformed line", () => {
    const { session } = fakeServer();
    let reason = "";
    session.onClose((r) => (reason = r));
    session.receive("{not json");
    expect(reason).toMatch(/^malformed message from notesd/);
  });

  it("rejects a call whose line cannot be sent", async () => {
    const session = new JsonRpcSession(async () => {
      throw new Error("not connected to notesd");
    });
    await expect(new Client(session).vault.list()).rejects.toThrow("not connected to notesd");
  });
});
