// The editors on screen by tab, so the editor's commands (save, search)
// reach the active tab's editor, and the panes that follow it (the
// Outline) hear when an editor comes, goes or changes.
import type { EditorView } from "@codemirror/view";

import type { NoteDoc } from "./document";

/** Editor is a tab's editor: its view and its document. */
export interface Editor {
  view: EditorView;
  doc: NoteDoc;
}

/** Editors maps tab IDs to their editors. */
export class Editors {
  private readonly byTab = new Map<string, Editor>();
  private readonly listeners = new Set<() => void>();
  private changes = 0;

  set(tabId: string, editor: Editor): () => void {
    this.byTab.set(tabId, editor);
    this.changed();
    return () => {
      if (this.byTab.get(tabId) !== editor) return;
      this.byTab.delete(tabId);
      this.changed();
    };
  }

  get(tabId: string): Editor | undefined {
    return this.byTab.get(tabId);
  }

  /** changed tells subscribers an editor's text or selection changed. */
  changed(): void {
    this.changes++;
    for (const fn of this.listeners) fn();
  }

  /** subscribe calls fn on every change; it returns the unsubscribe. */
  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  /** version counts the changes, for useSyncExternalStore. */
  version = (): number => this.changes;
}
