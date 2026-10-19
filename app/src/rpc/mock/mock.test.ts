import { describe, expect, it } from "vitest";

import { Client, ErrorCode, type Event, PROTOCOL } from "../gen/api";
import { RPCError } from "../transport";
import { MockTransport, mockVaults } from "./mock";

const now = () => new Date("2026-10-16T12:00:00Z");

async function connected(mock = new MockTransport(mockVaults(), { now })) {
  const client = new Client(mock);
  await client.rpc.hello({ protocol: PROTOCOL, client: "test" });
  return { mock, client };
}

/** nextEvent resolves with the next event the transport delivers. */
function nextEvent(mock: MockTransport): Promise<Event> {
  return new Promise((resolve) => {
    const off = mock.onEvent((e) => {
      off();
      resolve(e);
    });
  });
}

describe("MockTransport", () => {
  it("answers rpc.hello and refuses other calls before it", async () => {
    const client = new Client(new MockTransport());
    await expect(client.vault.list()).rejects.toEqual(new RPCError(ErrorCode.notReady, "call rpc.hello first"));
    await expect(client.rpc.hello({ protocol: 2, client: "test" })).rejects.toHaveProperty(
      "code",
      ErrorCode.protocolMismatch,
    );
    await expect(client.rpc.hello({ protocol: PROTOCOL, client: "test" })).resolves.toEqual({
      protocol: 1,
      server: "notesd mock",
      caller: "local",
    });
  });

  it("answers as a session inside lodge: who it is, what it may call, its writes' author", async () => {
    const mock = new MockTransport(mockVaults(), { now, caller: "session", login: "ana@example.com", node: "phone" });
    const client = new Client(mock);
    await expect(client.rpc.hello({ protocol: PROTOCOL, client: "web" })).resolves.toEqual({
      protocol: 1,
      server: "notesd mock",
      caller: "session",
      login: "ana@example.com",
      node: "phone",
    });
    await expect(client.rpc.ping()).resolves.toBeUndefined();
    await expect(client.vault.list()).resolves.toHaveLength(3);
    await expect(client.vault.add({ path: "/home/demo/x" })).rejects.toHaveProperty("code", ErrorCode.methodNotFound);
    await expect(client.sync.hubs()).rejects.toHaveProperty("code", ErrorCode.methodNotFound);
    await client.note.create({ vaultId: 1, path: "From the phone.md", content: "hi\n", source: "agent", author: "x" });
    const [v] = await client.note.versions({ vaultId: 1, path: "From the phone.md" });
    expect(v).toMatchObject({ source: "editor", author: "phone (web)" });
  });

  it("lists vaults by name, ignoring case", async () => {
    const { client } = await connected();
    expect((await client.vault.list()).map((v) => v.name)).toEqual(["garden", "Notes", "Work"]);
  });

  it("adds a vault and emits vault.added after subscribe", async () => {
    const { mock, client } = await connected();
    await expect(client.events.subscribe()).resolves.toEqual({ seq: 0, resync: false });
    const event = nextEvent(mock);
    const added = await client.vault.add({ path: "/home/demo/Recipes/" });
    expect(added).toEqual({
      id: 4,
      name: "Recipes",
      path: "/home/demo/Recipes",
      createdAt: "2026-10-16T12:00:00.000Z",
    });
    await expect(event).resolves.toEqual({ seq: 1, event: "vault.added", data: { vault: added } });
    expect((await client.vault.list()).map((v) => v.id)).toEqual([3, 1, 4, 2]);
  });

  it("refuses relative paths, overlapping vaults and a second subscription", async () => {
    const { client } = await connected();
    await expect(client.vault.add({ path: "Notes" })).rejects.toHaveProperty("code", ErrorCode.invalidParams);
    await expect(client.vault.add({ path: "/home/demo/Notes/daily" })).rejects.toHaveProperty(
      "code",
      ErrorCode.conflict,
    );
    await expect(client.vault.add({ path: "/home/demo" })).rejects.toHaveProperty("code", ErrorCode.conflict);
    await expect(client.vault.add({ path: "/home/demo/Notes2", name: "Two" })).resolves.toHaveProperty("name", "Two");
    await client.events.subscribe();
    await expect(client.events.subscribe()).rejects.toHaveProperty("code", ErrorCode.conflict);
  });

  it("makes a vault's folder with create, as notesd does", async () => {
    const { client } = await connected();
    await expect(client.vault.add({ path: "/home/demo/Garden", create: true })).resolves.toHaveProperty(
      "path",
      "/home/demo/Garden",
    );
    for (const [path, code] of [
      ["/home/demo/Garden", ErrorCode.conflict],
      ["/home/demo/Notes", ErrorCode.conflict],
      ["/home/demo/Documents/Work/Inner", ErrorCode.conflict],
      ["/home/missing/Vault", ErrorCode.invalidParams],
    ] as const) {
      await expect(client.vault.add({ path, create: true }), path).rejects.toHaveProperty("code", code);
    }
  });

  it("removes a vault and emits vault.removed", async () => {
    const { mock, client } = await connected();
    await client.events.subscribe();
    const event = nextEvent(mock);
    await expect(client.vault.remove({ id: 2 })).resolves.toBeUndefined();
    await expect(event).resolves.toEqual({ seq: 1, event: "vault.removed", data: { id: 2 } });
    await expect(client.vault.remove({ id: 2 })).rejects.toHaveProperty("code", ErrorCode.notFound);
    expect((await client.vault.list()).map((v) => v.id)).toEqual([3, 1]);
  });

  it("fails calls after close", async () => {
    const { mock, client } = await connected();
    let reason = "";
    mock.onClose((r) => (reason = r));
    mock.close("gone");
    expect(reason).toBe("gone");
    await expect(client.vault.list()).rejects.toThrow("notesd connection closed: gone");
  });
});
