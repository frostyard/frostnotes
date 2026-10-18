// What the status bar shows (docs/specs/ui.md#status-bar): the vault's
// sync state in words, the active note's backlinks, read again whenever
// the vault's files change, and its words and characters, or its
// selection's, counted once typing pauses.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { useEffect, useState, useSyncExternalStore } from "react";

import { useFiles } from "../data/files";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import type { DocumentRegistry } from "../editor/registry";
import { markdownSupport } from "../editor/syntax/index";
import { countWords } from "../editor/words";
import type { StatusCounts, StatusSync } from "../features/status/StatusBar";
import type { SyncStatus } from "../rpc/gen/api";
import { activeTab } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";

/** statusSync is the status bar's sync item for a vault's sync state. */
export function statusSync(s: SyncStatus): StatusSync {
  return { state: s.state, text: syncText(s), conflicts: s.conflicts };
}

function syncText(s: SyncStatus): string {
  switch (s.state) {
    case "idle":
      return s.syncedAt
        ? `Synced ${new Date(s.syncedAt).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`
        : "Synced";
    case "syncing":
      return "Syncing…";
    case "offline":
      return `Offline: ${s.error ?? "the hub cannot be reached"}`;
    case "revoked":
      return s.error ?? "This device was removed from the hub.";
    case "reset":
      return s.error ?? "The hub was reset: join it again.";
  }
}

/** useBacklinkCount counts the notes that link to path; null without one or while unknown. */
export function useBacklinkCount(path: string | null): number | null {
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  const [count, setCount] = useState<number | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger: any change in the vault may add or drop a link
  useEffect(() => {
    if (path === null || vaultId === null) {
      setCount(null);
      return;
    }
    let stale = false;
    client.note
      .backlinks({ vaultId, path })
      .then((links) => !stale && setCount(new Set(links.map((l) => l.source)).size))
      .catch(() => !stale && setCount(null));
    return () => {
      stale = true;
    };
  }, [client, vaultId, path, files]);
  return count;
}

/** COUNT_DELAY is how long after the last change the counts are taken again, in ms. */
export const COUNT_DELAY = 150;

/** countsOf counts a note's words and characters, or its selection's while there is one. */
export function countsOf(state: EditorState): StatusCounts {
  // Comments are found from the syntax tree: parse as far as 100 ms allows.
  ensureSyntaxTree(state, state.doc.length, 100);
  const ranges = state.selection.ranges.filter((r) => !r.empty);
  if (ranges.length === 0) return countWords(state);
  const out = { words: 0, characters: 0 };
  for (const r of ranges) {
    const c = countWords(state, r.from, r.to);
    out.words += c.words;
    out.characters += c.characters;
  }
  return out;
}

/** useNoteCounts counts the active note: its editor's state, or the note's text in the reading view; null without a note. */
export function useNoteCounts(docs: DocumentRegistry): StatusCounts | null {
  const changes = useSyncExternalStore(docs.editors.subscribe, docs.editors.version);
  const tab = useWorkspace((s) => activeTab(s.ws));
  const vaultId = useNotes((s) => s.vaultId);
  const [counts, setCounts] = useState<StatusCounts | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: changes is the trigger: an editor's text or selection changed
  useEffect(() => {
    const view = tab.view;
    if (view.type !== "note" || vaultId === null) {
      setCounts(null);
      return;
    }
    const timer = setTimeout(() => {
      const editor = docs.editors.get(tab.id);
      if (editor) {
        setCounts(countsOf(editor.view.state));
        return;
      }
      const doc = docs.find(vaultId, view.path);
      setCounts(
        doc?.isLoaded
          ? countsOf(EditorState.create({ doc: doc.text.toString(), extensions: markdownSupport() }))
          : null,
      );
    }, COUNT_DELAY);
    return () => clearTimeout(timer);
  }, [docs, tab, vaultId, changes]);
  return counts;
}
