// The editors on screen by tab, so the editor's commands (save, search)
// reach the active tab's editor.
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

  set(tabId: string, editor: Editor): () => void {
    this.byTab.set(tabId, editor);
    return () => {
      if (this.byTab.get(tabId) === editor) this.byTab.delete(tabId);
    };
  }

  get(tabId: string): Editor | undefined {
    return this.byTab.get(tabId);
  }
}
