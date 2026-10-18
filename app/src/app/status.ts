// What the status bar shows (docs/specs/ui.md#status-bar): the vault's
// sync state in words, and the active note's backlinks, read again
// whenever the vault's files change.
import { useEffect, useState } from "react";

import { useFiles } from "../data/files";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import type { StatusSync } from "../features/status/StatusBar";
import type { SyncStatus } from "../rpc/gen/api";

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
