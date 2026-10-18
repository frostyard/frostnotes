// M3's exit evidence 2 (docs/plans/0005-m3-desktop-app.md, Phase 7): every
// hotkey in the spec's keyboard map runs its command in the built app. The
// app runs over the mock's vault; before each press the driver lays out a
// workspace where every bound command applies (a note in the editor with
// history both ways, its cursor on a link, more tabs, a closed tab), then
// asks the harness to press the keys, which reach the page as the system
// delivers them, and records what the window's registry ran.
// workspace:close-window is recorded, not run, until the end, when the
// driver closes the window and the harness checks the app exits.
import type { EditorView } from "@codemirror/view";

import { CommandRegistry } from "../commands/registry";
import {
  closeMenu,
  closeMove,
  closePreview,
  closePrompt,
  closeSettings,
  closeTemplates,
  useOverlays,
} from "../data/overlays";
import { setRenaming } from "../data/panes";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { formatHotkey, HOTKEY_NAMES, type HotkeyCommand, hotkeysFor } from "../lib/keymap";
import { emptyWorkspace, type ViewState, type Workspace } from "../workspace/model";
import { activeTab, back, close, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { drive, editors, frames, say, showApp, until } from "./drive";

/** NOT_YET are the keyboard map's commands M3 does not have: the graph view is M5's. */
const NOT_YET: ReadonlySet<string> = new Set(["graph:open"]);

/** The note the editor shows, and the note its link leads to. */
const NOTE = "Welcome.md";
const LINK = "[[Projects/Seed list]]";

interface Run {
  id: string;
  ran: boolean;
}

export interface HotkeyResult {
  command: string;
  keys: string;
  /** Whether the command applied before the press: the layout was right. */
  applied: boolean;
  /** Whether the registry ran the command when the keys arrived. */
  ran: boolean;
  /** Every command the press ran. */
  runs: string[];
  /** The key presses the page saw, as KeyboardEvent names them; "handled" when something before the window's listener took it. */
  seen: string[];
}

const note = (path: string): ViewState => ({ type: "note", path, mode: "edit" });

/** layout is the workspace every press starts from: NOTE active with history both ways, three more tabs, a closed one. */
function layout(current: Workspace): Workspace {
  let w: Workspace = { ...emptyWorkspace(), nextId: current.nextId, left: current.left, right: current.right };
  const empty = activeTab(w).id;
  w = open(w, note("Projects/Seed list.md"), "tab");
  w = close(w, empty);
  w = open(w, note("Daily/2026-10-17.md"), "tab");
  w = open(w, note("Projects/Seed list.md"), "tab");
  w = close(w, activeTab(w).id);
  w = open(w, note("Projects/Orchard plan.md"), "tab");
  w = open(w, note(NOTE), "here");
  w = open(w, note("Projects/Seed list.md"), "here");
  return back(w);
}

/** prepare closes what the last press opened, lays the workspace out, and puts the cursor on a new link at the note's end. */
async function prepare(): Promise<EditorView> {
  closeMenu();
  closeMove();
  closePreview();
  closePrompt();
  closeSettings();
  closeTemplates();
  setRenaming(null);
  (document.activeElement as HTMLElement | null)?.blur();
  const before = new Set(editors());
  useWorkspace.getState().update(layout);
  let view: EditorView | undefined;
  try {
    await until(
      () => {
        view = editors().find((v) => !before.has(v) && v.state.doc.toString().includes("# Welcome"));
        return view !== undefined;
      },
      5000,
      `the editor on ${NOTE}`,
    );
  } catch (err) {
    const shown = editors().map((v) => JSON.stringify(v.state.doc.sliceString(0, 20)));
    throw new Error(
      `${String(err)}; the active tab shows ${JSON.stringify(activeTab(useWorkspace.getState().ws).view)}, the editors ${shown.join(", ")}`,
    );
  }
  const v = view as EditorView;
  const end = v.state.doc.length;
  const lead = v.state.doc.sliceString(end - 1) === "\n" ? "" : "\n";
  v.dispatch({ changes: { from: end, insert: `${lead}${LINK}\n` }, selection: { anchor: end + lead.length + 3 } });
  v.focus();
  await frames(2);
  return v;
}

/** runHotkeys presses every binding of the keyboard map, in its order. */
export function runHotkeys(root: HTMLElement): Promise<void> {
  // What reaches the page: the window's first key listener, so it sees
  // what the editor or a field took before the app's listener.
  const seen: string[] = [];
  window.addEventListener("keydown", (e) => {
    if (["Control", "Shift", "Alt", "Meta"].includes(e.key)) return;
    const mods = `${e.ctrlKey ? "Ctrl+" : ""}${e.altKey ? "Alt+" : ""}${e.shiftKey ? "Shift+" : ""}`;
    seen.push(`${mods}${e.key} (${e.code})${e.defaultPrevented ? " handled" : ""}`);
  });
  // The registry records what it runs; the window has one, made by MainWindow.
  const runs: Run[] = [];
  const pending: Promise<unknown>[] = [];
  let registry: CommandRegistry | null = null;
  const proto = CommandRegistry.prototype;
  const register = proto.register;
  const run = proto.run;
  proto.register = function (this: CommandRegistry, ...commands) {
    registry = this;
    return register.apply(this, commands);
  };
  proto.run = function (this: CommandRegistry, id: string): boolean {
    const c = this.get(id);
    const ran = c !== undefined && (c.applies?.() ?? true);
    runs.push({ id, ran });
    if (ran && c && id !== "workspace:close-window") {
      pending.push(Promise.resolve(c.run()).catch((err: unknown) => console.error(`command ${id}:`, err)));
    }
    return ran;
  };
  // The mock's first vault by name is empty; its notes are in Notes.
  usePrefs.getState().setPrefs({ lastVault: "/home/demo/Notes" });
  showApp(root, async () => {
    const { MockTransport, mockFiles, mockVaults } = await import("../rpc/mock/mock");
    return new MockTransport(mockVaults(), { latency: 15, files: mockFiles() });
  });
  return drive(
    "hotkeys",
    async () => {
      await until(() => useNotes.getState().vaultId !== null && registry !== null, 20_000, "the mock's vault");
      const results: HotkeyResult[] = [];
      for (const command of Object.keys(HOTKEY_NAMES) as HotkeyCommand[]) {
        for (const hotkey of hotkeysFor(command)) {
          await prepare();
          const applied = (registry as CommandRegistry | null)?.applies(command) ?? false;
          runs.length = 0;
          pending.length = 0;
          seen.length = 0;
          await say({ say: "press", modifiers: hotkey.modifiers, key: hotkey.key });
          try {
            await until(() => runs.length > 0, NOT_YET.has(command) ? 1500 : 5000, `${command} to run`);
          } catch {
            // Nothing ran: recorded below.
          }
          await Promise.all(pending);
          await frames(2);
          results.push({
            command,
            keys: formatHotkey(hotkey),
            applied,
            ran: runs.some((r) => r.id === command && r.ran),
            runs: runs.filter((r) => r.ran).map((r) => r.id),
            seen: [...seen],
          });
        }
      }
      const failed = results.filter((r) => (NOT_YET.has(r.command) ? r.ran : !r.ran));
      return {
        ok: failed.length === 0,
        bindings: results.length,
        ran: results.filter((r) => r.ran).length,
        notYet: [...NOT_YET],
        failed: failed.map((r) => `${r.keys} ${r.command}`),
        notices: useOverlays.getState().notices.map((n) => n.text),
        results,
      };
      // Last, Close window for real: the app must exit.
    },
    () => {
      closePrompt();
      closeSettings();
      if (registry) run.call(registry, "workspace:close-window");
    },
  );
}
