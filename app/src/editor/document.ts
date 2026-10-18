// A note's document (ADR-0022; docs/design/app.md#documents-saves-and-merges):
// its text, its base (the hash notesd last confirmed) and the text that
// base names, shared by every view showing the note. It saves 1 s after
// the last change and at most 10 s after the first, one write in flight;
// a merged answer, and another writer's change, apply as one remote
// transaction mapped over what was typed since, kept out of the history.
import {
  Annotation,
  ChangeSet,
  EditorState,
  type Extension,
  type SelectionRange,
  Text,
  Transaction,
  type TransactionSpec,
} from "@codemirror/state";

import { type Client, ErrorCode, type WriteResult } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { changesBetween } from "./diff";

/** remote marks a change another writer made, or a merge: never the user's, never in the history. */
export const remote = Annotation.define<boolean>();

/** forwarded marks a change copied from another view of the same document. */
export const forwarded = Annotation.define<boolean>();

/** SAVE_DELAY and SAVE_MAX are the debounce: 1 s after the last change, at most 10 s after the first. */
export const SAVE_DELAY = 1000;
export const SAVE_MAX = 10_000;

/** RETRY_FIRST and RETRY_MAX bound a failed save's retries: 2, 4, 8 … 60 s. */
export const RETRY_FIRST = 2000;
export const RETRY_MAX = 60_000;

/** DocView is what a document needs of a view: its state, and a way to change it. */
export interface DocView {
  readonly state: EditorState;
  dispatch(spec: TransactionSpec): void;
}

/** Notice is what a document's tabs say about it (docs/specs/ui.md#notices-in-a-tab). */
export type Notice =
  /** A merge kept the other side of some hunks; the writer's text is version `version`. */
  | { kind: "conflict"; conflicts: number; by: string; version?: number }
  /** notesd refused the base; the document now holds the file, and text what the editor held. */
  | { kind: "refused"; text: string }
  /** A save failed and retries. */
  | { kind: "failed"; reason: string }
  /** The note was deleted and nothing was unsaved. */
  | { kind: "deleted" };

/** DocStatus is where the document stands. */
export type DocStatus = "loading" | "missing" | "clean" | "dirty" | "saving";

/** DocSnapshot is what React renders of a document. */
export interface DocSnapshot {
  status: DocStatus;
  notice: Notice | null;
  path: string;
}

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

/** NoteDoc is one open note. */
export class NoteDoc {
  text: Text = Text.empty;
  /** The hash notesd last confirmed: the next write's base. */
  base = "";
  /** The text base names. */
  baseText = "";
  private readonly views = new Set<DocView>();
  private loaded = false;
  private missing = false;
  private dirty = false;
  private inFlight: { sent: Text; since: ChangeSet; done: Promise<void> } | null = null;
  /** A note.changed hash that arrived while a write was in flight. */
  private seen: string | null = null;
  private noticeValue: Notice | null = null;
  private debounce: ReturnType<typeof setTimeout> | undefined;
  private maxWait: ReturnType<typeof setTimeout> | undefined;
  private retry: ReturnType<typeof setTimeout> | undefined;
  private retryDelay = RETRY_FIRST;
  private readonly listeners = new Set<() => void>();
  private snap: DocSnapshot;
  private disposed = false;

  constructor(
    readonly vaultId: number,
    public path: string,
    private readonly client: () => Client | null,
  ) {
    this.snap = { status: "loading", notice: null, path };
  }

  // --- React

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  snapshot = (): DocSnapshot => this.snap;

  private notify(): void {
    const status: DocStatus = !this.loaded
      ? this.missing
        ? "missing"
        : "loading"
      : this.inFlight
        ? "saving"
        : this.dirty
          ? "dirty"
          : "clean";
    this.snap = { status, notice: this.noticeValue, path: this.path };
    for (const fn of this.listeners) fn();
  }

  get notice(): Notice | null {
    return this.noticeValue;
  }

  get unsaved(): boolean {
    return this.dirty || this.inFlight !== null;
  }

  private setNotice(n: Notice | null): void {
    this.noticeValue = n;
    this.notify();
  }

  /** dismiss clears the notice. */
  dismiss(): void {
    this.setNotice(null);
  }

  // --- loading and views

  /** load reads the note. */
  async load(): Promise<void> {
    const c = this.client();
    if (!c) return;
    try {
      const n = await c.note.read({ vaultId: this.vaultId, path: this.path });
      if (this.loaded || this.disposed) return;
      this.text = Text.of(n.content.split("\n"));
      this.base = n.hash;
      this.baseText = n.content;
      this.loaded = true;
      this.missing = false;
    } catch (err) {
      if (err instanceof RPCError && err.code === ErrorCode.notFound) this.missing = true;
      else this.noticeValue = { kind: "failed", reason: message(err) };
    }
    this.notify();
  }

  get isLoaded(): boolean {
    return this.loaded;
  }

  /** createState is a state for a new view of the document. */
  createState(extensions: Extension, selection?: SelectionRange | { anchor: number; head?: number }): EditorState {
    const clamp = (n: number) => Math.max(0, Math.min(n, this.text.length));
    return EditorState.create({
      doc: this.text,
      extensions,
      selection: selection
        ? { anchor: clamp(selection.anchor), head: clamp(selection.head ?? selection.anchor) }
        : undefined,
    });
  }

  attach(view: DocView): void {
    this.views.add(view);
  }

  /** detach forgets a view; the last one going saves at once. */
  detach(view: DocView): void {
    this.views.delete(view);
    if (this.views.size === 0) void this.save();
  }

  /**
   * local takes a transaction a view applied: a user's edit is the
   * document's change, forwarded to the other views and saved soon.
   */
  local(tr: Transaction, from: DocView): void {
    if (!tr.docChanged || tr.annotation(remote) || tr.annotation(forwarded)) return;
    this.text = tr.state.doc;
    if (this.inFlight) this.inFlight.since = this.inFlight.since.compose(tr.changes);
    const userEvent = tr.annotation(Transaction.userEvent);
    for (const v of this.views) {
      if (v === from) continue;
      v.dispatch({
        changes: tr.changes,
        annotations: [
          forwarded.of(true),
          Transaction.addToHistory.of(false),
          ...(userEvent ? [Transaction.userEvent.of(userEvent)] : []),
        ],
      });
    }
    this.markDirty();
  }

  private markDirty(): void {
    const first = !this.dirty;
    this.dirty = true;
    clearTimeout(this.debounce);
    this.debounce = setTimeout(() => void this.save(), SAVE_DELAY);
    if (first && this.maxWait === undefined) this.maxWait = setTimeout(() => void this.save(), SAVE_MAX);
    if (first) this.notify();
  }

  // applyRemote applies a change to every view, out of their histories.
  private applyRemote(changes: ChangeSet): void {
    if (changes.empty) return;
    for (const v of this.views) {
      v.dispatch({ changes, annotations: [remote.of(true), Transaction.addToHistory.of(false)] });
    }
    this.text = changes.apply(this.text);
  }

  // --- saving

  /** save writes the text now, unless nothing is unsaved or a write is in flight; it resolves when this write is answered. */
  async save(): Promise<void> {
    if (!this.loaded || !this.dirty || this.disposed) return this.inFlight?.done;
    if (this.inFlight) return this.inFlight.done;
    clearTimeout(this.debounce);
    clearTimeout(this.maxWait);
    clearTimeout(this.retry);
    this.debounce = this.maxWait = this.retry = undefined;
    const c = this.client();
    if (!c) {
      this.retryLater("not connected to notesd");
      return;
    }
    const sent = this.text;
    let finish = () => {};
    const done = new Promise<void>((resolve) => {
      finish = resolve;
    });
    this.inFlight = { sent, since: ChangeSet.empty(sent.length), done };
    this.dirty = false;
    this.notify();
    try {
      const res = await c.note.write({
        vaultId: this.vaultId,
        path: this.path,
        base: this.base,
        content: sent.toString(),
      });
      this.answered(res);
    } catch (err) {
      await this.failed(err);
    } finally {
      finish();
    }
  }

  /** flush saves now and waits until nothing is unsaved, or a failed save waits to retry. */
  async flush(): Promise<void> {
    while (this.loaded && !this.disposed) {
      if (this.inFlight) {
        await this.inFlight.done;
        continue;
      }
      if (!this.dirty || this.retry !== undefined) return;
      await this.save();
    }
  }

  /** retryNow runs a waiting retry at once. */
  retryNow(): void {
    clearTimeout(this.retry);
    this.retry = undefined;
    void this.save();
  }

  private answered(res: WriteResult): void {
    const flight = this.inFlight;
    this.inFlight = null;
    if (!flight) return;
    this.base = res.hash;
    this.retryDelay = RETRY_FIRST;
    if (this.noticeValue?.kind === "failed") this.noticeValue = null;
    if (res.merged && res.content !== undefined) {
      this.applyRemote(changesBetween(flight.sent.toString(), res.content).map(flight.since));
      this.baseText = res.content;
      if (res.conflicts > 0) {
        this.noticeValue = {
          kind: "conflict",
          conflicts: res.conflicts,
          by: res.mergedAuthor || res.mergedSource || "another writer",
          version: res.conflictVersion,
        };
      }
    } else {
      this.baseText = flight.sent.toString();
    }
    if (!flight.since.empty) this.dirty = true;
    this.notify();
    if (this.dirty) void this.save();
    const seen = this.seen;
    this.seen = null;
    if (seen !== null && seen !== this.base) this.changed(seen);
  }

  private async failed(err: unknown): Promise<void> {
    this.inFlight = null;
    this.dirty = true;
    if (err instanceof RPCError && err.code === ErrorCode.notFound) {
      await this.removed();
      return;
    }
    // notesd has no base to merge from ("has no version … to merge from",
    // "… is gone"); a file that kept changing as it was written retries.
    if (err instanceof RPCError && err.code === ErrorCode.conflict && /no version|is gone/.test(err.message)) {
      await this.refused();
      return;
    }
    this.retryLater(message(err));
  }

  private retryLater(reason: string): void {
    this.noticeValue = { kind: "failed", reason };
    clearTimeout(this.retry);
    this.retry = setTimeout(() => {
      this.retry = undefined;
      void this.save();
    }, this.retryDelay);
    this.retryDelay = Math.min(this.retryDelay * 2, RETRY_MAX);
    this.notify();
  }

  // refused: notesd no longer has the base, so it cannot merge. The
  // document takes the file's text, and the editor's is kept for the merge
  // view, so nothing typed is lost and nothing is overwritten.
  private async refused(): Promise<void> {
    const held = this.text.toString();
    const c = this.client();
    if (!c) return this.retryLater("not connected to notesd");
    try {
      const n = await c.note.read({ vaultId: this.vaultId, path: this.path });
      this.applyRemote(changesBetween(this.text.toString(), n.content));
      this.base = n.hash;
      this.baseText = n.content;
      this.dirty = false;
      this.noticeValue = { kind: "refused", text: held };
      this.notify();
    } catch (err) {
      this.retryLater(message(err));
    }
  }

  // --- other writers

  /** changed handles note.changed: another writer's content, unless it is the base. */
  changed(hash: string): void {
    if (!this.loaded || hash === this.base) return;
    if (this.inFlight) {
      this.seen = hash;
      return;
    }
    if (this.dirty) {
      void this.save(); // notesd merges what is unsaved into the new content
      return;
    }
    void this.reload();
  }

  /** reload reads the note and applies the difference as another writer's change. */
  async reload(): Promise<void> {
    const c = this.client();
    if (!c || !this.loaded) return;
    try {
      const n = await c.note.read({ vaultId: this.vaultId, path: this.path });
      if (this.inFlight || this.dirty) {
        if (this.dirty) void this.save();
        return;
      }
      if (n.hash === this.base) return;
      this.applyRemote(changesBetween(this.text.toString(), n.content));
      this.base = n.hash;
      this.baseText = n.content;
      if (this.noticeValue?.kind === "deleted") this.noticeValue = null;
      this.notify();
    } catch {
      // A note that is gone says so through note.removed.
    }
  }

  /** reconnected catches up after the connection came back: saves what is unsaved, else reads again. */
  reconnected(): void {
    if (!this.loaded) {
      void this.load();
      return;
    }
    if (this.dirty) void this.save();
    else void this.reload();
  }

  /** moved follows the note to a new path. */
  moved(to: string): void {
    this.path = to;
    this.notify();
  }

  /**
   * removed handles note.removed: unsaved text creates the note again, as
   * an edit beats a delete (ADR-0005); otherwise the tabs say it is gone.
   */
  async removed(): Promise<void> {
    if (!this.loaded) return;
    if (this.inFlight) await this.inFlight.done;
    if (!this.dirty) {
      this.setNotice({ kind: "deleted" });
      return;
    }
    const c = this.client();
    if (!c) return this.retryLater("not connected to notesd");
    const text = this.text.toString();
    try {
      const res = await c.note.create({ vaultId: this.vaultId, path: this.path, content: text });
      this.base = res.hash;
      this.baseText = text;
      this.dirty = this.text.toString() !== text;
      this.noticeValue = null;
      this.notify();
      if (this.dirty) void this.save();
    } catch (err) {
      this.retryLater(message(err));
    }
  }

  /** restore brings a deleted note back from its last version. */
  async restore(): Promise<void> {
    const c = this.client();
    if (!c) return;
    const versions = await c.note.versions({ vaultId: this.vaultId, path: this.path });
    const last = versions.find((v) => !v.deleted);
    if (!last) return;
    const res = await c.note.revert({ vaultId: this.vaultId, path: this.path, version: last.id, base: "" });
    const n = await c.note.read({ vaultId: this.vaultId, path: this.path });
    this.applyRemote(changesBetween(this.text.toString(), n.content));
    this.base = res.hash;
    this.baseText = n.content;
    this.dirty = false;
    this.setNotice(null);
  }

  dispose(): void {
    this.disposed = true;
    clearTimeout(this.debounce);
    clearTimeout(this.maxWait);
    clearTimeout(this.retry);
    this.listeners.clear();
  }
}
