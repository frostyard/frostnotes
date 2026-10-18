import { describe, expect, it } from "vitest";

import type { Event, VaultFile } from "../rpc/gen/api";
import { applyFileEvent, emptyFiles, fromListing } from "./files";

const at = () => "2026-10-17T09:30:00Z";
const file = (path: string, hash: string): VaultFile => ({
  path,
  kind: path.endsWith(".md") ? "note" : "attachment",
  hash,
  size: 10,
  modifiedAt: "2026-10-01T00:00:00Z",
});
const listed = fromListing(1, { files: [file("a.md", "h1"), file("Old/b.md", "h2")], folders: ["Old", "Spare"] }, 5);

const changed = (seq: number, path: string, hash: string, vaultId = 1): Event => ({
  seq,
  event: "note.changed",
  data: { vaultId, path, hash, source: "editor" },
});

describe("applyFileEvent", () => {
  it("adds and changes files, with their folders", () => {
    let s = applyFileEvent(listed, changed(6, "New/Deep/c.png", "h3"), at);
    expect(s.files.get("New/Deep/c.png")).toEqual({
      path: "New/Deep/c.png",
      kind: "attachment",
      hash: "h3",
      size: 0,
      modifiedAt: at(),
    });
    expect(s.folders).toEqual(["New", "New/Deep", "Old", "Spare"]);
    s = applyFileEvent(s, changed(7, "a.md", "h4"), at);
    expect(s.files.get("a.md")).toMatchObject({ hash: "h4", size: 10, modifiedAt: at() });
  });

  it("moves and removes files, keeping their folders", () => {
    let s = applyFileEvent(
      listed,
      { seq: 6, event: "note.moved", data: { vaultId: 1, from: "Old/b.md", to: "b.txt" } },
      at,
    );
    expect([...s.files.keys()]).toEqual(["a.md", "b.txt"]);
    expect(s.files.get("b.txt")).toMatchObject({ kind: "attachment", hash: "h2" });
    s = applyFileEvent(s, { seq: 7, event: "note.removed", data: { vaultId: 1, path: "a.md" } }, at);
    expect([...s.files.keys()]).toEqual(["b.txt"]);
    expect(s.folders).toEqual(["Old", "Spare"]);
  });

  it("changes nothing for what the listing reflects, another vault, or what it has", () => {
    expect(applyFileEvent(listed, changed(5, "z.md", "h9"), at)).toBe(listed);
    expect(applyFileEvent(listed, changed(6, "z.md", "h9", 2), at)).toBe(listed);
    expect(applyFileEvent(listed, changed(6, "a.md", "h1"), at)).toBe(listed);
    expect(applyFileEvent(listed, { seq: 6, event: "note.removed", data: { vaultId: 1, path: "z.md" } }, at)).toBe(
      listed,
    );
    expect(applyFileEvent(emptyFiles(null), changed(6, "a.md", "h1"), at)).toEqual(emptyFiles(null));
  });

  it("ends where the disk is when events after the listing's start are applied over it", () => {
    // The listing was taken after seq 6 and 7 happened; applying every event
    // after its start (5) in order arrives at the same state.
    const later = fromListing(1, { files: [file("a.md", "h3")], folders: [] }, 5);
    const s = [changed(6, "a.md", "h2"), changed(7, "a.md", "h3")].reduce((x, e) => applyFileEvent(x, e, at), later);
    expect(s.files.get("a.md")?.hash).toBe("h3");
  });
});
