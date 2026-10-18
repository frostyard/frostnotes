// The document's saves and merges (ADR-0022), with each writer pair the
// ADR names: the editor and an outside edit, a pull, an agent's write, and
// a second tab. Views here are headless (a state and a dispatch), as the
// document needs no more; merges are the mock's, scripted line by line.
import { history, undo, undoDepth } from "@codemirror/commands";
import { type EditorState, Transaction, type TransactionSpec } from "@codemirror/state";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Client, PROTOCOL } from "../rpc/gen/api";
import { type MergeInput, MockTransport, mockVaults } from "../rpc/mock/mock";
import type { Transport } from "../rpc/transport";
import { type DocView, type NoteDoc, RETRY_FIRST, SAVE_DELAY, SAVE_MAX } from "./document";
import { DocumentRegistry } from "./registry";

/** TestView is a headless view of a document. */
class TestView implements DocView {
  state: EditorState;
  constructor(
    private readonly doc: NoteDoc,
    at?: number,
  ) {
    this.state = doc.createState([history()], at === undefined ? undefined : { anchor: at });
    doc.attach(this);
  }
  dispatch(spec: TransactionSpec | Transaction): void {
    const tr = spec instanceof Transaction ? spec : this.state.update(spec);
    this.state = tr.state;
    this.doc.local(tr, this);
  }
  type(text: string, at = this.state.selection.main.head): void {
    this.dispatch({
      changes: { from: at, insert: text },
      selection: { anchor: at + text.length },
      userEvent: "input.type",
    });
  }
  undo(): void {
    undo({ state: this.state, dispatch: (tr) => this.dispatch(tr) });
  }
  get text(): string {
    return this.state.doc.toString();
  }
  get cursor(): number {
    return this.state.selection.main.head;
  }
}

/** lineMerge merges texts of equal line counts line by line; a line both sides changed keeps the head's and conflicts. */
function lineMerge({ base, head, incoming }: MergeInput) {
  const [b, h, i] = [base, head, incoming].map((t) => t.split("\n")) as [string[], string[], string[]];
  if (b.length !== h.length || b.length !== i.length) return undefined;
  let conflicts = 0;
  const content = b
    .map((line, k) => {
      const hl = h[k] ?? "";
      const il = i[k] ?? "";
      if (hl !== line && il !== line && hl !== il) {
        conflicts++;
        return hl;
      }
      return hl !== line ? hl : il;
    })
    .join("\n");
  return { content, conflicts };
}

const NOTE = "one\ntwo\nthree\n";

/** run awaits p while running fake timers, for a mock that answers after a latency. */
async function run<T>(p: Promise<T>): Promise<T> {
  let done = false;
  void p.finally(() => {
    done = true;
  });
  while (!done) await vi.advanceTimersByTimeAsync(10);
  return p;
}

async function setup(opts: { latency?: number; transport?: (mock: MockTransport) => Transport } = {}) {
  const mock = new MockTransport(mockVaults(), { latency: opts.latency, files: { 1: { "n.md": NOTE } } });
  mock.scriptMerges(lineMerge);
  const client = new Client(opts.transport?.(mock) ?? mock);
  await run(client.rpc.hello({ protocol: PROTOCOL, client: "frostnotes-app" }));
  const registry = new DocumentRegistry(() => client);
  mock.onEvent((e) => registry.event(e));
  await run(client.events.subscribe({}));
  const doc = registry.acquire(1, "n.md");
  await vi.waitFor(() => expect(doc.isLoaded).toBe(true));
  const writes = () => mock.calls.filter((c) => c.method === "note.write");
  return { mock, client, registry, doc, writes };
}

/** settle runs timers and the mock's replies until nothing is left to run. */
async function settle(ms = 0) {
  await vi.advanceTimersByTimeAsync(ms);
  for (let i = 0; i < 20; i++) await vi.advanceTimersByTimeAsync(0);
}

beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] }));
afterEach(() => vi.useRealTimers());

describe("saving", () => {
  it("saves 1 s after the last change, and at most 10 s after the first", async () => {
    const { doc, writes, mock } = await setup();
    const view = new TestView(doc, 3);
    view.type("!");
    await settle(SAVE_DELAY - 100);
    view.type("!");
    await settle(SAVE_DELAY - 100);
    expect(writes()).toHaveLength(0);
    await settle(100);
    expect(writes()).toHaveLength(1);
    expect(mock.vault(1).text("n.md")).toBe("one!!\ntwo\nthree\n");
    expect(doc.snapshot().status).toBe("clean");

    // Typing that never pauses still saves after SAVE_MAX.
    for (let t = 0; t < SAVE_MAX; t += 500) {
      view.type(".");
      await settle(500);
    }
    expect(writes()).toHaveLength(2);
  });

  it("keeps one write in flight; what is typed meanwhile goes in the next, on the new base", async () => {
    const { doc, writes, mock } = await setup({ latency: 50 });
    const view = new TestView(doc, 3);
    view.type("!");
    await settle(SAVE_DELAY);
    expect(doc.snapshot().status).toBe("saving");
    view.type("?");
    await settle(50); // the first answer; the second save starts at once
    expect(writes()).toHaveLength(2);
    const [first, second] = writes().map((w) => w.params as { base: string });
    expect(second?.base).not.toBe(first?.base);
    await settle(100);
    expect(mock.vault(1).text("n.md")).toBe("one!?\ntwo\nthree\n");
    expect(doc.snapshot().status).toBe("clean");
    expect(writes().map((w) => (w.params as { content: string }).content)).toEqual([
      "one!\ntwo\nthree\n",
      "one!?\ntwo\nthree\n",
    ]);
  });
});

describe("writer pairs", () => {
  it("an outside edit while typing merges, the cursor stays, and undo undoes only the typing", async () => {
    const { doc, mock } = await setup();
    const view = new TestView(doc, NOTE.indexOf("three") + 5);
    view.type("!");
    mock.vault(1).outside("n.md", "ONE\ntwo\nthree\n");
    await settle();
    expect(view.text).toBe("ONE\ntwo\nthree!\n");
    expect(mock.vault(1).text("n.md")).toBe("ONE\ntwo\nthree!\n");
    expect(view.cursor).toBe(view.text.indexOf("!") + 1);
    view.undo();
    expect(view.text).toBe("ONE\ntwo\nthree\n");
  });

  it("a pull while nothing is unsaved applies as another writer's change, out of the history", async () => {
    const { doc, mock } = await setup();
    const view = new TestView(doc, NOTE.indexOf("two"));
    mock.vault(1).outside("n.md", "zero\none\ntwo\nthree\n", "sync");
    await settle();
    expect(view.text).toBe("zero\none\ntwo\nthree\n");
    expect(view.cursor).toBe(view.text.indexOf("two"));
    expect(undoDepth(view.state)).toBe(0);
  });

  it("an agent's append reaches a note being edited, and both stay", async () => {
    const { doc, mock, client } = await setup();
    const view = new TestView(doc, 3);
    view.type(" more");
    const agent = new Client(mock);
    await agent.rpc.hello({ protocol: PROTOCOL, client: "claude-code" });
    const n = await client.note.read({ vaultId: 1, path: "n.md" });
    mock.scriptMerges((m) => ({ content: `${m.incoming}${m.head.slice(m.base.length)}`, conflicts: 0 }));
    await agent.note.write({
      vaultId: 1,
      path: "n.md",
      base: n.hash,
      content: `${NOTE}appended by the agent\n`,
      source: "agent",
    });
    await settle();
    expect(view.text).toBe("one more\ntwo\nthree\nappended by the agent\n");
    expect(mock.vault(1).text("n.md")).toBe(view.text);
  });

  it("a second tab shows the same document, saves once, and undoes only its own typing", async () => {
    const { doc, writes } = await setup();
    const a = new TestView(doc, 3);
    const b = new TestView(doc, 0);
    a.type("!");
    expect(b.text).toBe("one!\ntwo\nthree\n");
    b.type(">");
    expect(a.text).toBe(">one!\ntwo\nthree\n");
    await settle(SAVE_DELAY);
    expect(writes()).toHaveLength(1);
    b.undo();
    expect(a.text).toBe("one!\ntwo\nthree\n");
    expect(b.text).toBe(a.text);
  });
});

describe("what goes wrong", () => {
  it("a conflict keeps the newer side, and says who wrote it and where the editor's text went", async () => {
    const { doc, mock, client } = await setup();
    const view = new TestView(doc, 3);
    view.type(" (mine)");
    mock.vault(1).outside("n.md", "one (theirs)\ntwo\nthree\n");
    await settle();
    expect(view.text).toBe("one (theirs)\ntwo\nthree\n");
    const notice = doc.notice;
    expect(notice).toMatchObject({ kind: "conflict", conflicts: 1, by: "outside" });
    const kept = await client.note.readVersion({
      vaultId: 1,
      version: notice?.kind === "conflict" ? (notice.version ?? 0) : 0,
    });
    expect(kept.content).toBe("one (mine)\ntwo\nthree\n");
  });

  it("a base notesd refuses takes the file's text and keeps the editor's for the merge view", async () => {
    const { doc, mock } = await setup();
    const view = new TestView(doc, 3);
    doc.base = "f".repeat(64); // a version notesd no longer has
    view.type(" lost?");
    await settle(SAVE_DELAY);
    expect(view.text).toBe(NOTE);
    expect(doc.notice).toEqual({ kind: "refused", text: "one lost?\ntwo\nthree\n" });
    expect(mock.vault(1).text("n.md")).toBe(NOTE);
  });

  it("a failed save stays unsaved, retries after 2, then 4 s, and clears when it lands", async () => {
    let failures = 2;
    const { doc, mock } = await setup({
      transport: (m) => ({
        call: (method, params) =>
          method === "note.write" && failures-- > 0
            ? Promise.reject(new Error("notesd connection closed: gone"))
            : m.call(method, params),
        onEvent: (h) => m.onEvent(h),
        onClose: (h) => m.onClose(h),
      }),
    });
    const view = new TestView(doc, 3);
    view.type("!");
    await settle(SAVE_DELAY);
    expect(doc.notice).toEqual({ kind: "failed", reason: "notesd connection closed: gone" });
    expect(doc.unsaved).toBe(true);
    await settle(RETRY_FIRST);
    expect(failures).toBe(0);
    await settle(RETRY_FIRST * 2);
    expect(doc.notice).toBeNull();
    expect(mock.vault(1).text("n.md")).toBe("one!\ntwo\nthree\n");
  });

  it("a delete with unsaved text creates the note again; without, the tabs say it is gone", async () => {
    const { doc, mock, registry } = await setup();
    const view = new TestView(doc, 3);
    view.type(" kept");
    mock.vault(1).outside("n.md", null);
    await settle();
    expect(mock.vault(1).text("n.md")).toBe("one kept\ntwo\nthree\n");
    expect(doc.notice).toBeNull();

    mock.vault(1).outside("n.md", null);
    await settle();
    expect(doc.notice).toEqual({ kind: "deleted" });
    await doc.restore();
    await settle();
    expect(mock.vault(1).text("n.md")).toBe("one kept\ntwo\nthree\n");
    expect(registry.find(1, "n.md")?.notice).toBeNull();
  });

  it("follows a move", async () => {
    const { doc, registry, mock } = await setup();
    const view = new TestView(doc, 3);
    mock.vault(1).move("n.md", "Moved/n.md");
    await settle();
    expect(registry.find(1, "Moved/n.md")).toBe(doc);
    view.type("!");
    await settle(SAVE_DELAY);
    expect(mock.vault(1).text("Moved/n.md")).toBe("one!\ntwo\nthree\n");
  });
});
