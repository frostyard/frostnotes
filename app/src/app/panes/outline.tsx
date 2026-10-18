// The Outline pane's container (docs/specs/ui.md, Right sidebar panes):
// the active note's headings, from its editor's syntax tree as the user
// types (150 ms after the text last changed), or from its text in the
// reading view; the heading the cursor is under is marked. A click moves
// the editor's cursor to the heading, or scrolls the reading view to it.
import { EditorView } from "@codemirror/view";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";

import { toggleFolded, usePanes } from "../../data/panes";
import { useNotes } from "../../data/stores";
import { headingsOf, headingsOfText } from "../../editor/outline";
import { useDocuments } from "../../editor/registry";
import { OutlinePane } from "../../features/outline/OutlinePane";
import { buildOutline, type Heading, headingAt, outlineRows } from "../../lib/outline";
import { activeTab } from "../../workspace/ops";
import { useWorkspace } from "../../workspace/store";

/** OUTLINE_DELAY is how long after the text last changed the outline reads its headings again. */
export const OUTLINE_DELAY = 150;

/** OutlineContainer draws the Outline pane for the active note. */
export function OutlineContainer() {
  const docs = useDocuments();
  useSyncExternalStore(docs.editors.subscribe, docs.editors.version);
  const vaultId = useNotes((s) => s.vaultId);
  const tab = useWorkspace((s) => activeTab(s.ws));
  const note = tab.view.type === "note" ? tab.view : null;
  const editor = note?.mode === "edit" ? docs.editors.get(tab.id) : undefined;
  const state = editor?.view.state;
  const doc = state?.doc;
  const text = note && !editor && vaultId !== null ? docs.find(vaultId, note.path)?.text.toString() : undefined;

  const [headings, setHeadings] = useState<{ path: string; list: Heading[] } | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: doc is the trigger: the headings follow the text
  useEffect(() => {
    if (!note) return;
    const read = () => {
      const list = state ? headingsOf(state) : text !== undefined ? headingsOfText(text) : [];
      setHeadings({ path: note.path, list });
    };
    // A new note reads at once; typing waits for a pause.
    if (headings?.path !== note.path) {
      read();
      return;
    }
    const timer = setTimeout(read, OUTLINE_DELAY);
    return () => clearTimeout(timer);
  }, [note?.path, doc, text]);

  const folded = usePanes((s) => (note ? s.folded[note.path] : undefined));
  const list = headings && note && headings.path === note.path ? headings.list : [];
  const rows = useMemo(() => outlineRows(buildOutline(list), new Set(folded ?? [])), [list, folded]);
  const head = state?.selection.main.head;
  const current = useMemo(() => {
    if (head === undefined) return -1;
    const at = headingAt(list, head);
    return at < 0 ? -1 : rows.findIndex((r) => r.heading === list[at]);
  }, [list, rows, head]);

  if (!note) return <OutlinePane rows={null} current={-1} onToggle={() => {}} onGo={() => {}} />;
  return (
    <OutlinePane
      rows={rows}
      current={current}
      onToggle={(from) => toggleFolded(note.path, from)}
      onGo={(h) => {
        if (editor) {
          const at = Math.min(h.from, editor.view.state.doc.length);
          editor.view.dispatch({ selection: { anchor: at }, effects: EditorView.scrollIntoView(at, { y: "start" }) });
          editor.view.focus();
          return;
        }
        // The reading view's headings carry their line (docs/specs/render.md).
        const line = (text ?? "").slice(0, h.from).split("\n").length;
        document
          .querySelector(`[data-testid="reading-view"] [data-line="${line}"]`)
          ?.scrollIntoView({ block: "start" });
      }}
    />
  );
}
