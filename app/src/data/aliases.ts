// The vault's notes' aliases (docs/design/app.md#data), for the quick
// switcher and link completion: note.list's entries carry them. Read when
// the window's vault opens and again a moment after its files change,
// since an edit may change a note's aliases.
import { useEffect } from "react";
import { create } from "zustand";

import { useFiles } from "./files";
import { useClient } from "./session";
import { useNotes } from "./stores";

/** AliasesState is a vault's notes' aliases by path. */
export interface AliasesState {
  vaultId: number | null;
  byPath: ReadonlyMap<string, readonly string[]>;
}

export const useAliases = create<AliasesState>(() => ({ vaultId: null, byPath: new Map() }));

/** ALIASES_DELAY is how long after the vault's files change the aliases are read again, in ms. */
export const ALIASES_DELAY = 1000;

/** aliasesOf is a note's aliases; [] for a note without any or a file that is not a note. */
export function aliasesOf(path: string): readonly string[] {
  return useAliases.getState().byPath.get(path) ?? [];
}

/** useAliasesSync keeps useAliases for the window's vault. */
export function useAliasesSync(): void {
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger: an edit may change a note's aliases
  useEffect(() => {
    if (vaultId === null) {
      useAliases.setState({ vaultId: null, byPath: new Map() });
      return;
    }
    let stale = false;
    const read = () =>
      client.note
        .list({ vaultId })
        .then((entries) => {
          if (stale) return;
          const byPath = new Map<string, readonly string[]>();
          for (const e of entries) if (e.aliases.length > 0) byPath.set(e.path, e.aliases);
          useAliases.setState({ vaultId, byPath });
        })
        .catch(() => {});
    // The first listing at once; later ones once changes settle.
    const timer = setTimeout(read, useAliases.getState().vaultId === vaultId ? ALIASES_DELAY : 0);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [client, vaultId, files]);
}
