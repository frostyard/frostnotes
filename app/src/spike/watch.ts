// The desktop's half of M4's exit evidence 2
// (docs/plans/0006-m4-notes-inside-lodge.md, Phase 7): a phone edit shows
// in the desktop app's editor within 2 s. The built app connects to a
// device notesd that syncs with the hub behind lodge, opens Phone.md, and
// says the editor's text each time it changes; the harness
// (internal/webevidence) types in the web app at phone size and times each
// edit from its last keystroke to the line that holds it. Done.md ends it.
import { feed } from "../data/events";
import type { Connect } from "../data/session";
import { useNotes } from "../data/stores";
import { open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { drive, editors, say, showApp, until } from "./drive";

/** NOTE is the note the phone edits; DONE, once it changes, ends the run. */
export const NOTE = "Phone.md";
export const DONE = "Done.md";

/** POLL is how often the editor's text is read, in ms: the measurement's resolution. */
const POLL = 5;

/** runWatch opens the note and says its editor's text as it changes, until Done.md does. */
export function runWatch(root: HTMLElement, connect: Connect): Promise<void> {
  showApp(root, connect);
  return drive("watch", async () => {
    await until(
      () => useNotes.getState().vaultId !== null && useWorkspace.getState().vaultPath !== null,
      30_000,
      "notesd's vault",
    );
    let done = false;
    feed.listen((e) => {
      if (e.event === "note.changed" && e.data.path === DONE) done = true;
    });
    useWorkspace.getState().update((w) => open(w, { type: "note", path: NOTE, mode: "edit" }, "here"));
    await until(() => editors().length > 0, 30_000, `${NOTE} in an editor`);
    let last = editors()[0]?.state.doc.toString() ?? "";
    await say({ say: "ready", text: last });
    let lines = 1;
    while (!done) {
      await new Promise((r) => setTimeout(r, POLL));
      const text = editors()[0]?.state.doc.toString();
      if (text === undefined || text === last) continue;
      last = text;
      lines++;
      await say({ say: "text", text });
    }
    return { lines };
  });
}
