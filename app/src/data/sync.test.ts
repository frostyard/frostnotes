// The app's sync flow (docs/design/sync.md#pairing-and-sessions): the steps
// of `notesctl hub join` with no terminal, pairing again, and leaving, over
// the mock.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode, PROTOCOL } from "../rpc/gen/api";
import { MockTransport, type MockOptions, mockVaults } from "../rpc/mock/mock";
import { RPCError } from "../rpc/transport";
import { ensurePaired, hubURL, joinHub, leaveHub, pairAgain, syncError } from "./sync";

const now = () => new Date("2026-10-28T09:00:00Z");
const HUB = "http://127.0.0.1:7788";

async function connected(opts: MockOptions = {}) {
  const mock = new MockTransport(mockVaults(), { now, ...opts });
  const client = new Client(mock);
  await client.rpc.hello({ protocol: PROTOCOL, client: "test" });
  return { mock, client };
}

/** syncCalls is the sync methods the mock answered, in order. */
const syncCalls = (mock: MockTransport) => mock.calls.map((c) => c.method).filter((m) => m.startsWith("sync."));

/** codes collects the pairing codes the flow shows. */
function codes() {
  const shown: string[] = [];
  return { shown, onCode: (code: string) => shown.push(code) };
}

describe("joinHub", () => {
  it("pairs with a new hub, waits for the approval and joins", async () => {
    const { mock, client } = await connected();
    const { shown, onCode } = codes();
    const status = await joinHub(client, { vaultId: 1, url: `${HUB}/`, name: "shared", device: "laptop" }, onCode);
    expect(shown).toEqual(["ABCD-EFGH"]);
    expect(status).toMatchObject({ vaultId: 1, url: HUB, hubVault: "shared", state: "idle" });
    expect(syncCalls(mock)).toEqual(["sync.hubs", "sync.pair", "sync.paired", "sync.join"]);
    expect(mock.calls.find((c) => c.method === "sync.pair")?.params).toEqual({ url: HUB, name: "laptop" });
  });

  it("joins a hub this device paired with before, with no pairing", async () => {
    const { mock, client } = await connected();
    await joinHub(client, { vaultId: 1, url: HUB, name: "shared" }, codes().onCode);
    mock.calls.length = 0;
    const status = await joinHub(client, { vaultId: 2, url: `${HUB}//`, name: "work" }, codes().onCode);
    expect(status).toMatchObject({ vaultId: 2, hubVault: "work" });
    expect(syncCalls(mock)).toEqual(["sync.hubs", "sync.join"]);
  });

  it("pairs again once when the hub refuses the token, then joins", async () => {
    const { mock, client } = await connected();
    await joinHub(client, { vaultId: 1, url: HUB, name: "shared" }, codes().onCode);
    mock.calls.length = 0;
    mock.refuseJoins(1);
    const { shown, onCode } = codes();
    const status = await joinHub(client, { vaultId: 2, url: HUB, name: "work" }, onCode);
    expect(shown).toEqual(["ABCD-EFGH"]);
    expect(status).toMatchObject({ vaultId: 2, hubVault: "work" });
    expect(syncCalls(mock)).toEqual(["sync.hubs", "sync.join", "sync.pair", "sync.paired", "sync.join"]);
  });

  it("stops after one pairing when the hub refuses the token again", async () => {
    const { mock, client } = await connected();
    await joinHub(client, { vaultId: 1, url: HUB, name: "shared" }, codes().onCode);
    mock.calls.length = 0;
    mock.refuseJoins(2);
    await expect(joinHub(client, { vaultId: 2, url: HUB, name: "work" }, codes().onCode)).rejects.toHaveProperty(
      "code",
      ErrorCode.revoked,
    );
    expect(mock.calls.filter((c) => c.method === "sync.pair")).toHaveLength(1);
    expect(mock.calls.filter((c) => c.method === "sync.join")).toHaveLength(2);
  });

  it("says the pairing expired when the code is not approved", async () => {
    const { client } = await connected({ pairingExpired: true });
    await expect(joinHub(client, { vaultId: 1, url: HUB, name: "shared" }, codes().onCode)).rejects.toThrow(
      "The pairing expired: the code was not approved within 10 minutes.",
    );
  });

  it("stops between calls when the person cancels", async () => {
    const { mock, client } = await connected();
    const controller = new AbortController();
    const { shown, onCode } = codes();
    const going = joinHub(
      client,
      { vaultId: 1, url: HUB, name: "shared" },
      (code) => {
        onCode(code);
        controller.abort();
      },
      controller.signal,
    );
    await expect(going).rejects.toHaveProperty("name", "AbortError");
    expect(shown).toEqual(["ABCD-EFGH"]);
    expect(syncCalls(mock)).toEqual(["sync.hubs", "sync.pair"]);
  });

  it("does nothing when cancelled before it starts", async () => {
    const { mock, client } = await connected();
    const controller = new AbortController();
    controller.abort();
    await expect(
      joinHub(client, { vaultId: 1, url: HUB, name: "shared" }, codes().onCode, controller.signal),
    ).rejects.toHaveProperty("name", "AbortError");
    expect(syncCalls(mock)).toEqual([]);
  });
});

describe("ensurePaired", () => {
  it("reports whether the device was paired with the hub before", async () => {
    const { client } = await connected();
    expect(await ensurePaired(client, HUB, undefined, codes().onCode)).toBe(false);
    expect(await ensurePaired(client, HUB, "desk", codes().onCode)).toBe(true);
  });
});

describe("pairAgain", () => {
  it("replaces the hub's token with no join", async () => {
    const { mock, client } = await connected({ synced: [{ vaultId: 1, url: HUB, name: "shared", state: "revoked" }] });
    const { shown, onCode } = codes();
    await pairAgain(client, { url: `${HUB}/`, device: "desk" }, onCode);
    expect(shown).toEqual(["ABCD-EFGH"]);
    expect(syncCalls(mock)).toEqual(["sync.pair", "sync.paired"]);
    expect(mock.calls.find((c) => c.method === "sync.pair")?.params).toEqual({ url: HUB, name: "desk" });
  });
});

describe("leaveHub", () => {
  it("stops syncing the vault, which keeps its files", async () => {
    const { client } = await connected({ synced: [{ vaultId: 1, url: HUB, name: "shared" }] });
    expect(await client.sync.status({ vaultId: 1 })).toHaveLength(1);
    await leaveHub(client, 1);
    expect(await client.sync.status({ vaultId: 1 })).toEqual([]);
    expect((await client.vault.list()).map((v) => v.id)).toContain(1);
  });

  it("refuses a vault notesd does not have", async () => {
    const { client } = await connected();
    await expect(leaveHub(client, 99)).rejects.toHaveProperty("code", ErrorCode.notFound);
  });
});

describe("the flow's words", () => {
  it("takes a hub's URL trimmed, with no trailing slash", () => {
    expect(hubURL(" https://hub.example/ ")).toBe("https://hub.example");
    expect(hubURL("http://127.0.0.1:7788//")).toBe("http://127.0.0.1:7788");
  });

  it("says an error in notesd's own words", () => {
    expect(syncError(new RPCError(ErrorCode.conflict, "vault 2 syncs with a hub already"))).toBe(
      "vault 2 syncs with a hub already",
    );
    expect(syncError("gone")).toBe("gone");
  });
});
