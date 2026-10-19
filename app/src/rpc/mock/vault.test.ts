import { describe, expect, it } from "vitest";
import { sha256 } from "../../lib/sha256";
import { Client, ErrorCode, type Event, PROTOCOL } from "../gen/api";
import { MockTransport, mockVaults } from "./mock";

const now = () => new Date("2026-10-17T09:30:00Z");
const text = new TextEncoder();

async function connected(files: Record<string, string | Uint8Array> = {}) {
  const mock = new MockTransport(mockVaults(), { now, files: { 1: files } });
  const client = new Client(mock);
  await client.rpc.hello({ protocol: PROTOCOL, client: "test" });
  const events: Event[] = [];
  mock.onEvent((e) => events.push(e));
  await client.events.subscribe();
  return { mock, client, note: client.note, events };
}

/** settle lets the mock deliver the events of calls made so far. */
const settle = () => new Promise((r) => setTimeout(r, 0));

describe("sha256", () => {
  it("matches FIPS 180-4's vectors", () => {
    expect(sha256(new Uint8Array())).toBe("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
    expect(sha256(text.encode("abc"))).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    expect(sha256(text.encode("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"))).toBe(
      "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1",
    );
    expect(sha256(text.encode("a".repeat(1_000_000)))).toBe(
      "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0",
    );
  });
});

describe("the mock's note domain", () => {
  it("lists files and folders, with hashes", async () => {
    const { note } = await connected({ "a.md": "a\n", "Deep/b/c.png": "png", "Empty/": "" });
    const listing = await note.files({ vaultId: 1 });
    expect(listing.files.map((f) => [f.path, f.kind, f.size])).toEqual([
      ["Deep/b/c.png", "attachment", 3],
      ["a.md", "note", 2],
    ]);
    expect(listing.files[1]?.hash).toBe(sha256(text.encode("a\n")));
    expect(listing.folders).toEqual(["Deep", "Deep/b", "Empty"]);
    await expect(note.files({ vaultId: 9 })).rejects.toHaveProperty("code", ErrorCode.notFound);
  });

  it("writes with a base, merges a stale one and refuses a base that is no version", async () => {
    const { mock, note, events } = await connected({ "n.md": "one\n" });
    const n = await note.read({ vaultId: 1, path: "n.md" });
    const first = await note.write({ vaultId: 1, path: "n.md", base: n.hash, content: "one\ntwo\n" });
    expect(first).toEqual({ path: "n.md", hash: sha256(text.encode("one\ntwo\n")), merged: false, conflicts: 0 });

    // The base is behind, and the side since it did not change: the write takes the head.
    const merged = await note.write({ vaultId: 1, path: "n.md", base: n.hash, content: "one\n" });
    expect(merged).toMatchObject({ merged: true, content: "one\ntwo\n", conflicts: 0, mergedAuthor: "test" });

    mock.scriptMerges(({ head, incoming }) => ({ content: `${head}${incoming}`, conflicts: 0 }));
    const scripted = await note.write({ vaultId: 1, path: "n.md", base: n.hash, content: "three\n" });
    expect(scripted).toMatchObject({ merged: true, content: "one\ntwo\nthree\n", conflicts: 0 });
    expect(mock.vault(1).text("n.md")).toBe("one\ntwo\nthree\n");
    mock.scriptMerges(undefined);

    // Both sides changed: the newer side stands, and the writer's text is a conflict version.
    const conflict = await note.write({ vaultId: 1, path: "n.md", base: n.hash, content: "mine\n" });
    expect(conflict).toMatchObject({ merged: true, content: "one\ntwo\nthree\n", conflicts: 1 });
    const kept = await note.readVersion({ vaultId: 1, version: conflict.conflictVersion ?? 0 });
    expect(kept).toMatchObject({ content: "mine\n", version: { conflict: true, author: "test" } });

    await expect(note.write({ vaultId: 1, path: "n.md", base: "f".repeat(64), content: "x" })).rejects.toHaveProperty(
      "code",
      ErrorCode.conflict,
    );
    await expect(note.write({ vaultId: 1, path: "none.md", base: n.hash, content: "x" })).rejects.toHaveProperty(
      "code",
      ErrorCode.notFound,
    );
    await settle();
    expect(events.map((e) => e.event)).toEqual(["note.changed", "note.changed"]);
  });

  it("keeps every version and refuses a stale delete", async () => {
    const { mock, note } = await connected({ "n.md": "one\n" });
    mock.vault(1).outside("n.md", "two\n");
    const n = await note.read({ vaultId: 1, path: "n.md" });
    await expect(note.delete({ vaultId: 1, path: "n.md", base: sha256(text.encode("one\n")) })).rejects.toHaveProperty(
      "code",
      ErrorCode.conflict,
    );
    await note.delete({ vaultId: 1, path: "n.md", base: n.hash });
    const versions = await note.versions({ vaultId: 1, path: "n.md" });
    expect(versions.map((v) => [v.deleted, v.source])).toEqual([
      [true, "editor"],
      [false, "outside"],
      [false, "outside"],
    ]);
    await note.revert({ vaultId: 1, path: "n.md", version: versions[2]?.id ?? 0, base: "" });
    expect(mock.vault(1).text("n.md")).toBe("one\n");
  });

  it("refuses paths notesd refuses", async () => {
    const { note } = await connected();
    for (const path of ["", "/abs.md", "a/../b.md", ".hidden/x.md", "a//b.md"]) {
      await expect(note.create({ vaultId: 1, path, content: "" })).rejects.toHaveProperty(
        "code",
        ErrorCode.invalidParams,
      );
    }
  });

  it("moves and deletes folders as notesd does", async () => {
    const { mock, note, events } = await connected({ "Old/a.md": "a\n", "Old/b.md": "b\n", "Old/Empty/": "" });
    const moved = await note.moveFolder({ vaultId: 1, path: "Old", to: "New/Place" });
    expect(moved).toEqual({ path: "New/Place", moved: ["New/Place/a.md", "New/Place/b.md"], rewritten: [] });
    expect((await note.files({ vaultId: 1 })).folders).toEqual(["New", "New/Place", "New/Place/Empty"]);
    await settle();
    expect(events.filter((e) => e.event === "note.moved")).toHaveLength(2);

    const listed = (await note.files({ vaultId: 1 })).files.map((f) => ({ path: f.path, base: f.hash }));
    mock.vault(1).outside("New/Place/b.md", "b, edited\n");
    const res = await note.deleteFolder({ vaultId: 1, path: "New/Place", files: listed });
    expect(res).toEqual({ deleted: ["New/Place/a.md"], kept: ["New/Place/b.md"], removed: false });
    await expect(note.moveFolder({ vaultId: 1, path: "New", to: "New/Inner" })).rejects.toHaveProperty(
      "code",
      ErrorCode.invalidParams,
    );
  });

  it("resolves, renders, uploads and serves blobs", async () => {
    const { mock, note } = await connected({
      "Projects/Plan.md": "# Plan\n\nSee [[Seeds]] and [[Nowhere]].\n",
      "Seeds.md": "",
    });
    expect(await note.resolve({ vaultId: 1, from: "Projects/Plan.md", targets: ["Seeds", "Plan", "Nowhere"] })).toEqual(
      [{ target: "Seeds", path: "Seeds.md" }, { target: "Plan", path: "Projects/Plan.md" }, { target: "Nowhere" }],
    );
    const { html } = await note.render({ vaultId: 1, path: "Projects/Plan.md" });
    expect(html).toContain('<a class="internal-link" data-href="Seeds" data-path="Seeds.md">Seeds</a>');
    expect(html).toContain('class="internal-link is-unresolved"');

    const bytes = Uint8Array.of(0x89, 0x50, 0x4e, 0x47);
    const up = await note.upload({ vaultId: 1, path: "pic.png", data: btoa(String.fromCharCode(...bytes)) });
    expect(mock.blobURL(up.hash, "png")).toBe("data:image/png;base64,iVBORw==");
    await expect(note.upload({ vaultId: 1, path: "pic.png", data: "iVBORw==" })).rejects.toHaveProperty(
      "code",
      ErrorCode.conflict,
    );
  });

  it("replays durable events after sinceSeq, and asks for a resync from a seq it never had", async () => {
    const mock = new MockTransport(mockVaults(), { now, files: { 1: { "a.md": "a\n" } } });
    const writer = new Client(mock);
    await writer.rpc.hello({ protocol: PROTOCOL, client: "notesctl" });
    await writer.note.create({ vaultId: 1, path: "b.md", content: "b\n" });
    await writer.note.move({ vaultId: 1, path: "b.md", to: "c.md" });

    const replayed: Event[] = [];
    mock.onEvent((e) => replayed.push(e));
    const c = new Client(mock);
    const sub = await c.events.subscribe({ sinceSeq: 1 });
    await settle();
    expect(sub).toEqual({ seq: 2, resync: false });
    expect(replayed.map((e) => [e.seq, e.event])).toEqual([[2, "note.moved"]]);
  });

  it("emits vault.configured when settings change", async () => {
    const { client, events } = await connected();
    const s = await client.vault.configure({ id: 1, wikilinks: false });
    expect(s).toMatchObject({ wikilinks: false, sources: { wikilinks: "frostnotes", linkFormat: "default" } });
    await settle();
    expect(events).toEqual([{ seq: 1, event: "vault.configured", data: { id: 1 } }]);
  });
});
