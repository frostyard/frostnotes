// The history tab's container (docs/design/app.md#history-and-merges;
// docs/specs/ui.md#history-batch-2): the file's versions from note.versions
// and, for a synced vault, the hub's revisions from sync.history, each
// readable; Restore is note.revert with the document's base, and Compare
// opens the merge view. Batch 2's history list draws the list; until then
// it is plain.
import { useEffect, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useVaultState } from "../data/vaultState";
import { useNoteDoc } from "../editor/useNoteDoc";
import type { Revision, Version } from "../rpc/gen/api";
import type { MergeSide } from "../workspace/model";

export interface HistoryViewProps {
  vaultId: number;
  path: string;
  onCompare: (other: MergeSide) => void;
}

type Selected = { kind: "version"; v: Version } | { kind: "revision"; r: Revision };

const when = (at: string) => new Date(at).toLocaleString();

/** HistoryView lists a file's versions and the hub's revisions of it. */
export function HistoryView({ vaultId, path, onCompare }: HistoryViewProps) {
  const current = useClient();
  const client = useRef(current);
  client.current = current;
  const { doc } = useNoteDoc(vaultId, path);
  const syncs = useVaultState((s) => s.sync !== null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [revisions, setRevisions] = useState<Revision[]>([]);
  const [selected, setSelected] = useState<Selected | null>(null);
  const [content, setContent] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [generation, setGeneration] = useState(0);

  // biome-ignore lint/correctness/useExhaustiveDependencies: generation counts restores, after which the lists load again
  useEffect(() => {
    const c = client.current;
    let stale = false;
    void c.note
      .versions({ vaultId, path })
      .then((v) => !stale && setVersions(v))
      .catch((err: unknown) => !stale && setError(String(err instanceof Error ? err.message : err)));
    if (syncs) {
      void c.sync
        .history({ vaultId, path })
        .then((r) => !stale && setRevisions(r))
        .catch(() => {});
    }
    return () => {
      stale = true;
    };
  }, [vaultId, path, syncs, generation]);

  useEffect(() => {
    if (!selected) return;
    const c = client.current;
    let stale = false;
    const read =
      selected.kind === "version"
        ? c.note.readVersion({ vaultId, version: selected.v.id }).then((x) => x.content)
        : c.sync.readRevision({ vaultId, seq: selected.r.seq }).then((x) => x.content);
    read.then((t) => !stale && setContent(t)).catch((err: unknown) => !stale && setContent(String(err)));
    return () => {
      stale = true;
    };
  }, [selected, vaultId]);

  const restore = async (v: Version) => {
    try {
      const base = doc?.base ?? (await client.current.note.read({ vaultId, path })).hash;
      await client.current.note.revert({ vaultId, path, version: v.id, base });
      setGeneration((g) => g + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const row = (key: string, active: boolean, title: string, note: string, onClick: () => void) => (
    <li key={key}>
      <button
        type="button"
        onClick={onClick}
        aria-current={active || undefined}
        className={`w-full rounded-md px-3 py-1.5 text-left hover:bg-selection-inactive ${active ? "bg-selection-sidebar" : ""}`}
      >
        <span className="text-sidebar-row block">{title}</span>
        <span className="block text-[12px] leading-4 text-secondary">{note}</span>
      </button>
    </li>
  );

  return (
    <div className="flex h-full min-h-0">
      <nav aria-label="Versions" className="w-[280px] shrink-0 overflow-y-auto border-r border-separator p-2">
        {error && <p className="px-3 py-2 text-[12px] text-secondary">{error}</p>}
        <ul>
          {versions.map((v) =>
            row(
              `v${v.id}`,
              selected?.kind === "version" && selected.v.id === v.id,
              when(v.at),
              v.deleted ? "Deleted" : `${v.conflict ? "Conflict · " : ""}${v.author || v.source} · ${v.size} bytes`,
              () => setSelected({ kind: "version", v }),
            ),
          )}
        </ul>
        {revisions.length > 0 && (
          <>
            <h3 className="text-sidebar-section px-3 pt-3 pb-1 text-tertiary uppercase">On the hub</h3>
            <ul>
              {revisions.map((r) =>
                row(
                  `r${r.seq}`,
                  selected?.kind === "revision" && selected.r.seq === r.seq,
                  when(r.at),
                  `${r.deviceName} · ${r.op}`,
                  () => setSelected({ kind: "revision", r }),
                ),
              )}
            </ul>
          </>
        )}
      </nav>
      <section aria-label="Version" className="flex min-w-0 flex-1 flex-col">
        {selected && (
          <div className="flex h-9 shrink-0 items-center justify-end gap-3 border-b border-separator px-3 text-[12px]">
            {selected.kind === "version" && !selected.v.deleted && (
              <button type="button" className="text-accent hover:underline" onClick={() => void restore(selected.v)}>
                Restore this version
              </button>
            )}
            <button
              type="button"
              className="text-accent hover:underline"
              onClick={() =>
                onCompare(
                  selected.kind === "version"
                    ? { kind: "version", version: selected.v.id }
                    : { kind: "revision", seq: selected.r.seq },
                )
              }
            >
              Compare
            </button>
          </div>
        )}
        <pre className="min-h-0 flex-1 overflow-auto px-8 py-6 font-[inherit] text-[15px] leading-normal whitespace-pre-wrap select-text">
          {selected ? content : "Select a version."}
        </pre>
      </section>
    </div>
  );
}
