// A component's hold on a note's document: acquired from the registry for
// as long as the component shows the note, and its state as React reads it.
import { useEffect, useState, useSyncExternalStore } from "react";

import type { DocSnapshot, NoteDoc } from "./document";
import { useDocuments } from "./registry";

const LOADING: DocSnapshot = { status: "loading", notice: null, path: "" };
const noSubscribe = () => () => {};

/** useNoteDoc holds the document of the note at path while the component shows it. */
export function useNoteDoc(vaultId: number, path: string): { doc: NoteDoc | null; snap: DocSnapshot; ready: boolean } {
  const registry = useDocuments();
  const [doc, setDoc] = useState<NoteDoc | null>(null);
  useEffect(() => {
    const d = registry.acquire(vaultId, path);
    setDoc(d);
    return () => registry.release(d);
  }, [registry, vaultId, path]);
  const snap = useSyncExternalStore(doc?.subscribe ?? noSubscribe, doc?.snapshot ?? (() => LOADING));
  return { doc, snap, ready: doc !== null && snap.status !== "loading" && snap.status !== "missing" };
}
