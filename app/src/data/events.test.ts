import { describe, expect, it } from "vitest";

import type { Event } from "../rpc/gen/api";
import { EventFeed, SEQ_KEY } from "./events";
import { memoryStore } from "./storage";

const removed = (seq: number): Event => ({ seq, event: "note.removed", data: { vaultId: 1, path: "a.md" } });

describe("EventFeed", () => {
  it("starts from the first subscription and keeps how far it got", () => {
    const storage = memoryStore();
    const feed = new EventFeed(storage);
    expect(feed.since()).toBeUndefined();
    feed.started({ seq: 10, resync: false }, undefined);
    expect(feed.since()).toBe(10);
    const seen: number[] = [];
    feed.listen((e) => seen.push(e.seq ?? 0));
    feed.deliver(removed(11));
    feed.deliver({ event: "vault.scan", data: {} } as Event); // transient: no seq
    feed.deliver(removed(9)); // never goes back
    expect(seen).toEqual([11, 0, 9]);
    expect(storage.get(SEQ_KEY)).toBe("11");
    expect(new EventFeed(storage).since()).toBe(11);
  });

  it("resumes from what it kept, and starts again on a resync", () => {
    const storage = memoryStore({ [SEQ_KEY]: "40" });
    const feed = new EventFeed(storage);
    let resyncs = 0;
    feed.onResync(() => resyncs++);
    feed.started({ seq: 45, resync: false }, 40);
    expect(feed.since()).toBe(40); // replays carry it forward
    feed.started({ seq: 3, resync: true }, 40);
    expect(feed.since()).toBe(3);
    expect(resyncs).toBe(1);
    expect(new EventFeed(memoryStore({ [SEQ_KEY]: "nonsense" })).since()).toBeUndefined();
  });
});
