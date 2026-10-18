// The event feed (docs/design/app.md#data; ADR-0021): every event notesd
// sends, replays included, for what keeps state across runs (saved
// workspaces follow moves made while the app was closed), and the last
// durable sequence number applied, kept so the next run resumes after it.
import type { Event, Subscription } from "../rpc/gen/api";
import { type KeyValue, localStore } from "./storage";

/** SEQ_KEY is the local-storage key of the last applied sequence number. */
export const SEQ_KEY = "frostnotes.seq";

/** EventFeed delivers events to listeners and remembers how far it got. */
export class EventFeed {
  private readonly listeners = new Set<(e: Event) => void>();
  private readonly resyncs = new Set<() => void>();
  private last: number | undefined;

  constructor(private readonly storage: KeyValue) {
    const stored = storage.get(SEQ_KEY);
    const n = stored === null ? Number.NaN : Number(stored);
    this.last = Number.isSafeInteger(n) && n >= 0 ? n : undefined;
  }

  /** since is the sequence number to subscribe from: undefined before the first subscription. */
  since(): number | undefined {
    return this.last;
  }

  /**
   * started records a subscription made from since. When nothing will be
   * replayed (the first subscription, or notesd asks for a resync) the
   * feed starts from the subscription's sequence number, and a resync
   * tells the resync listeners that events were missed.
   */
  started(sub: Subscription, since: number | undefined): void {
    if (since === undefined || sub.resync) this.advance(sub.seq, true);
    if (sub.resync) for (const fn of this.resyncs) fn();
  }

  /** deliver passes e to every listener, then records its sequence number. */
  deliver(e: Event): void {
    for (const fn of this.listeners) fn(e);
    if (e.seq !== undefined) this.advance(e.seq, false);
  }

  /** listen subscribes fn to every event; it returns the unsubscribe. */
  listen(fn: (e: Event) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  /** onResync subscribes fn to resyncs, when events since the last run are lost. */
  onResync(fn: () => void): () => void {
    this.resyncs.add(fn);
    return () => this.resyncs.delete(fn);
  }

  private advance(seq: number, reset: boolean): void {
    if (!reset && this.last !== undefined && seq <= this.last) return;
    this.last = seq;
    this.storage.set(SEQ_KEY, String(seq));
  }
}

/** feed is the window's event feed. */
export const feed = new EventFeed(localStore());
