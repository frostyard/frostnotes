// How the panes open what they list: a row's click says where (a new tab
// with Ctrl or the middle button, to the right with Ctrl+Alt), and a
// backlink or a heading opens its note with the cursor at its line.
import { EditorView } from "@codemirror/view";

import type { DocumentRegistry } from "../../editor/registry";
import type { PaneClick } from "../../features/panes/common";
import type { Client } from "../../rpc/gen/api";
import { activeTab, type OpenWhere, open } from "../../workspace/ops";
import { useWorkspace } from "../../workspace/store";
import { viewOf } from "../commands";

/** whereOf is where a pane's row opens for a click. */
export function whereOf(click: PaneClick): OpenWhere {
  if (click.ctrl && click.alt) return "right";
  return click.ctrl || click.middle ? "tab" : "here";
}

/** openFile opens a vault file from a pane's row. */
export function openFile(path: string, click: PaneClick): void {
  useWorkspace.getState().update((w) => open(w, viewOf(path), whereOf(click)));
}

/**
 * openAt opens a note with its cursor at pos, and moves an editor that
 * already shows it there, scrolled to the middle.
 */
export function openAt(docs: DocumentRegistry, path: string, pos: number, click: PaneClick): void {
  useWorkspace.getState().update((w) => open(w, { type: "note", path, mode: "edit", cursor: pos }, whereOf(click)));
  requestAnimationFrame(() => {
    const tab = activeTab(useWorkspace.getState().ws);
    const ed = docs.editors.get(tab.id);
    if (!ed || ed.doc.path !== path) return;
    const at = Math.min(pos, ed.view.state.doc.length);
    ed.view.dispatch({ selection: { anchor: at }, effects: EditorView.scrollIntoView(at, { y: "center" }) });
    ed.view.focus();
  });
}

/** openAtLine opens a note at a line (1-based), reading the note for where the line starts. */
export async function openAtLine(
  c: Client,
  docs: DocumentRegistry,
  vaultId: number,
  path: string,
  line: number,
  click: PaneClick,
): Promise<void> {
  const doc = docs.find(vaultId, path);
  const text = doc?.isLoaded ? doc.text.toString() : (await c.note.read({ vaultId, path })).content;
  const lines = text.split("\n");
  const pos = lines.slice(0, Math.max(0, line - 1)).reduce((n, l) => n + l.length + 1, 0);
  openAt(docs, path, Math.min(pos, text.length), click);
}
