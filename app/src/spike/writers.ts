// M3's exit evidence 3 (docs/plans/0005-m3-desktop-app.md, Phase 7): each
// writer pair with the editor loses no token and keeps the cursor. The
// built app connects to a real notesd over the Tauri bridge, opens
// Writers.md in two tabs side by side, and asks the harness to type a
// token at the end of the line "tab1:" in the first and "tab2:" in the
// second, by turns, as keys the system delivers. While it types, the
// harness saves the note with vim, appends to it through MCP, and edits
// it on a second device whose changes come through the hub. Before each
// token the driver checks the tab's cursor is where its typing left it,
// the end of its line, whatever the others wrote. The harness checks the
// file and the second device hold every writer's tokens, in order.
import type { EditorState } from "@codemirror/state";

import { feed } from "../data/events";
import { useOverlays } from "../data/overlays";
import type { Connect } from "../data/session";
import { useNotes } from "../data/stores";
import { activeTab, open, split } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { drive, editors, say, showApp, until } from "./drive";

/** The note, the tabs' lines, and how many tokens each tab types. */
export const NOTE = "Writers.md";
export const LINES = ["tab1:", "tab2:"] as const;
export const TOKENS = 40;

/** QUIET is how long the note must stay unchanged, after every writer stopped, before the driver reports: longer than a save's debounce. */
const QUIET = 3000;

/** lineEnd is where the line starting with label ends, or -1. */
export function lineEnd(state: EditorState, label: string): number {
  for (let n = 1; n <= state.doc.lines; n++) {
    const line = state.doc.line(n);
    if (line.text.startsWith(label)) return line.to;
  }
  return -1;
}

/** label is tab k's line. */
const label = (k: number): string => LINES[k] ?? LINES[0];

/** token is tab k's token number i: a001, b001, … */
export function token(k: number, i: number): string {
  return `${"ab"[k]}${String(i).padStart(3, "0")}`;
}

interface Moved {
  tab: number;
  /** The token about to be typed. */
  before: string;
  head: number;
  want: number;
}

/** runWriters opens the note twice and types into both while the harness writes it from outside. */
export function runWriters(root: HTMLElement, connect: Connect): Promise<void> {
  showApp(root, connect);
  return drive("writers", async () => {
    await until(
      () => useNotes.getState().vaultId !== null && useWorkspace.getState().vaultPath !== null,
      30_000,
      "notesd's vault",
    );
    let done = false;
    feed.listen((e) => {
      if (e.event === "note.changed" && e.data.path === "Done.md") done = true;
    });
    useWorkspace.getState().update((w) => {
      const opened = open(w, { type: "note", path: NOTE, mode: "edit" }, "here");
      return split(opened, activeTab(opened).id, "right");
    });
    await until(
      () => editors().filter((v) => v.state.doc.toString().includes(LINES[1])).length === 2,
      30_000,
      `${NOTE} in two tabs`,
    );
    const views = editors().filter((v) => v.state.doc.toString().includes(LINES[1]));
    views.forEach((v, k) => {
      v.dispatch({ selection: { anchor: lineEnd(v.state, label(k)) } });
    });

    const moved: Moved[] = [];
    for (let i = 1; i <= TOKENS; i++) {
      for (const [k, v] of views.entries()) {
        const t = token(k, i);
        const want = lineEnd(v.state, label(k));
        const head = v.state.selection.main.head;
        if (head !== want) moved.push({ tab: k + 1, before: t, head, want });
        v.focus();
        await until(() => v.hasFocus, 5000, `tab ${k + 1}'s focus`);
        await say({ say: "type", text: ` ${t}` });
        await until(() => v.state.doc.toString().includes(` ${t}`), 15_000, `${t} to arrive`);
      }
    }
    await say({ say: "typed" });
    await until(() => done, 120_000, "the harness's Done.md");

    // Every writer has stopped: wait until the note stays still, saved.
    let text = views[0]?.state.doc.toString() ?? "";
    let still = performance.now();
    await until(
      () => {
        const now = views[0]?.state.doc.toString() ?? "";
        if (now !== text) {
          text = now;
          still = performance.now();
        }
        return performance.now() - still >= QUIET;
      },
      60_000,
      "the note to settle",
    );
    const tabs = views.map((v, k) => ({
      head: v.state.selection.main.head,
      want: lineEnd(v.state, label(k)),
      same: v.state.doc.toString() === text,
    }));
    return {
      ok: moved.length === 0 && tabs.every((t) => t.same && t.head === t.want),
      tokens: TOKENS,
      moved,
      tabs,
      notices: useOverlays.getState().notices.map((n) => n.text),
      text,
    };
  });
}
