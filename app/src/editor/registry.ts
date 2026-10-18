// The document registry (docs/design/app.md#documents-saves-and-merges):
// one NoteDoc per vault and path while any tab shows it, the events that
// reach it, and catching up after a reconnect.
import { createContext, useContext } from "react";

import type { Client, Event } from "../rpc/gen/api";
import { Editors } from "./active";
import { NoteDoc } from "./document";

const key = (vaultId: number, path: string) => `${vaultId}:${path}`;

/** DocumentRegistry holds the window's open documents. */
export class DocumentRegistry {
  private readonly docs = new Map<string, { doc: NoteDoc; refs: number }>();
  /** The editors on screen, by tab. */
  readonly editors = new Editors();

  constructor(private readonly client: () => Client | null) {}

  /** acquire returns a note's document, loading it when it is new; release it when done. */
  acquire(vaultId: number, path: string): NoteDoc {
    const k = key(vaultId, path);
    let entry = this.docs.get(k);
    if (!entry) {
      entry = { doc: new NoteDoc(vaultId, path, this.client), refs: 0 };
      this.docs.set(k, entry);
      void entry.doc.load();
    }
    entry.refs++;
    return entry.doc;
  }

  /**
   * release lets a document go once no tab holds it, after it saves. A tab
   * that takes it again at once (its note moved) keeps it.
   */
  release(doc: NoteDoc): void {
    const entry = this.docs.get(key(doc.vaultId, doc.path));
    if (!entry || entry.doc !== doc) return;
    entry.refs--;
    if (entry.refs > 0) return;
    queueMicrotask(() => {
      if (entry.refs > 0) return;
      // A document that cannot save yet stays, retrying, until it does.
      const settle = (): void => {
        const now = this.docs.get(key(doc.vaultId, doc.path));
        if (now !== entry || entry.refs > 0) return;
        if (doc.unsaved) {
          setTimeout(() => void doc.flush().finally(settle), 1000);
          return;
        }
        this.docs.delete(key(doc.vaultId, doc.path));
        doc.dispose();
      };
      void doc.flush().finally(settle);
    });
  }

  /** find returns an open document. */
  find(vaultId: number, path: string): NoteDoc | undefined {
    return this.docs.get(key(vaultId, path))?.doc;
  }

  /** event passes note events to the documents they concern. */
  event(e: Event): void {
    switch (e.event) {
      case "note.changed":
        this.find(e.data.vaultId, e.data.path)?.changed(e.data.hash);
        break;
      case "note.moved": {
        const k = key(e.data.vaultId, e.data.from);
        const entry = this.docs.get(k);
        if (!entry || this.docs.has(key(e.data.vaultId, e.data.to))) break;
        this.docs.delete(k);
        entry.doc.moved(e.data.to);
        this.docs.set(key(e.data.vaultId, e.data.to), entry);
        break;
      }
      case "note.removed":
        void this.find(e.data.vaultId, e.data.path)?.removed();
        break;
    }
  }

  /** reconnected catches every document up after the connection came back. */
  reconnected(): void {
    for (const { doc } of this.docs.values()) doc.reconnected();
  }

  /** unsaved reports whether any document holds text notesd has not confirmed. */
  unsaved(): boolean {
    return [...this.docs.values()].some(({ doc }) => doc.unsaved);
  }

  /** flushAll saves every document now. */
  async flushAll(): Promise<void> {
    await Promise.all([...this.docs.values()].map(({ doc }) => doc.flush()));
  }

  /** all lists the open documents. */
  all(): NoteDoc[] {
    return [...this.docs.values()].map((e) => e.doc);
  }
}

/** DocumentsContext gives editors the window's registry. */
export const DocumentsContext = createContext<DocumentRegistry | null>(null);

/** useDocuments returns the window's document registry. */
export function useDocuments(): DocumentRegistry {
  const r = useContext(DocumentsContext);
  if (!r) throw new Error("useDocuments outside a DocumentsContext");
  return r;
}
