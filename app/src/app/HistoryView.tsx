// The history tab's container (docs/design/app.md#history-and-merges;
// docs/specs/ui.md, History): the file's versions from note.versions and,
// for a synced vault, the hub's revisions from sync.history, drawn by
// HistoryList (T-0061); the selected one read-only with the editor's
// styles, or, with Show changes, how it differs from the current text.
// Restore is note.revert with the document's base; Compare opens the
// merge view.
import { unifiedMergeView } from "@codemirror/merge";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { useEffect, useMemo, useRef, useState } from "react";

import { notify } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { useVaultState } from "../data/vaultState";
import { markdownSupport } from "../editor/syntax/index";
import { editorTheme } from "../editor/theme";
import { useNoteDoc } from "../editor/useNoteDoc";
import { type HistoryEntry, HistoryList } from "../features/history/HistoryList";
import type { Revision, Version } from "../rpc/gen/api";
import type { MergeSide } from "../workspace/model";

export interface HistoryViewProps {
  vaultId: number;
  path: string;
  onCompare: (other: MergeSide) => void;
}

const versionEntry = (v: Version): HistoryEntry => ({
  id: `v${v.id}`,
  at: v.at,
  source: v.source,
  author: v.author,
  size: v.deleted ? null : v.size,
  conflict: v.conflict,
  deleted: v.deleted,
});

const revisionEntry = (r: Revision): HistoryEntry => ({
  id: `r${r.seq}`,
  at: r.at,
  source: "sync",
  author: r.deviceName,
  size: r.op === "delete" ? null : r.size,
  conflict: r.op === "conflict",
  deleted: r.op === "delete",
});

/** HistoryView shows a file's versions and the hub's revisions of it. */
export function HistoryView({ vaultId, path, onCompare }: HistoryViewProps) {
  const current = useClient();
  const client = useRef(current);
  client.current = current;
  const { doc, snap } = useNoteDoc(vaultId, path);
  const syncs = useVaultState((s) => s.sync !== null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [revisions, setRevisions] = useState<Revision[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState<string | null>(null);
  const [changes, setChanges] = useState(false);
  const [generation, setGeneration] = useState(0);

  // biome-ignore lint/correctness/useExhaustiveDependencies: generation counts restores, after which the lists load again
  useEffect(() => {
    const c = client.current;
    let stale = false;
    c.note
      .versions({ vaultId, path })
      .then((v) => !stale && setVersions(v))
      .catch((err: unknown) => !stale && notify(err instanceof Error ? err.message : String(err)));
    if (syncs) {
      c.sync
        .history({ vaultId, path })
        .then((r) => !stale && setRevisions(r))
        .catch(() => {});
    }
    return () => {
      stale = true;
    };
  }, [vaultId, path, syncs, generation]);

  const version = selected?.startsWith("v") ? versions.find((v) => `v${v.id}` === selected) : undefined;
  const revision = selected?.startsWith("r") ? revisions.find((r) => `r${r.seq}` === selected) : undefined;

  useEffect(() => {
    setContent(null);
    if (!version && !revision) return;
    let stale = false;
    const read = version
      ? client.current.note.readVersion({ vaultId, version: version.id }).then((x) => x.content)
      : client.current.sync.readRevision({ vaultId, seq: revision?.seq ?? 0 }).then((x) => x.content);
    read
      .then((t) => !stale && setContent(t))
      .catch((err: unknown) => !stale && setContent(`Cannot read it: ${err instanceof Error ? err.message : err}`));
    return () => {
      stale = true;
    };
  }, [vaultId, version, revision]);

  const restore = async (v: Version) => {
    try {
      const base = doc?.base ?? (await client.current.note.read({ vaultId, path })).hash;
      await client.current.note.revert({ vaultId, path, version: v.id, base });
      notify("Restored the version.");
      setGeneration((g) => g + 1);
    } catch (err) {
      notify(err instanceof Error ? err.message : String(err));
    }
  };

  const currentText = snap.status === "missing" || !doc?.isLoaded ? null : doc.text.toString();
  const entries = useMemo(() => versions.map(versionEntry), [versions]);
  const hubEntries = useMemo(() => (syncs ? revisions.map(revisionEntry) : null), [syncs, revisions]);
  const bar = "text-accent hover:underline disabled:text-tertiary disabled:no-underline";
  return (
    <div className="flex h-full min-h-0">
      <div className="w-[280px] shrink-0 overflow-y-auto border-r border-separator">
        <HistoryList versions={entries} revisions={hubEntries} selected={selected} onSelect={setSelected} />
      </div>
      <section aria-label="Version" className="flex min-w-0 flex-1 flex-col">
        {selected && (
          <div className="flex h-9 shrink-0 items-center justify-end gap-4 border-b border-separator px-3 text-[12px]">
            <button
              type="button"
              aria-pressed={changes}
              disabled={currentText === null}
              className={bar}
              onClick={() => setChanges(!changes)}
            >
              {changes ? "Hide changes" : "Show changes"}
            </button>
            {version && !version.deleted && (
              <button type="button" className={bar} onClick={() => void restore(version)}>
                Restore this version
              </button>
            )}
            <button
              type="button"
              className={bar}
              onClick={() =>
                onCompare(
                  version ? { kind: "version", version: version.id } : { kind: "revision", seq: revision?.seq ?? 0 },
                )
              }
            >
              Compare
            </button>
          </div>
        )}
        {selected === null ? (
          <p className="text-empty px-8 py-8 text-secondary">Select a version.</p>
        ) : content === null ? null : (
          <VersionText text={content} compareTo={changes ? currentText : null} />
        )}
      </section>
    </div>
  );
}

// VersionText shows a version read-only in the editor's styles, its
// difference from compareTo marked when given.
function VersionText({ text, compareTo }: { text: string; compareTo: string | null }) {
  const host = useRef<HTMLDivElement>(null);
  const readable = usePrefs((s) => s.readableLineLength);
  const fontSize = usePrefs((s) => s.fontSize);
  useEffect(() => {
    if (!host.current) return;
    const view = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: text,
        extensions: [
          markdownSupport(),
          editorTheme({ readable, fontSize }),
          EditorView.lineWrapping,
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          ...(compareTo === null
            ? []
            : [unifiedMergeView({ original: compareTo, mergeControls: false, highlightChanges: true })]),
        ],
      }),
    });
    return () => view.destroy();
  }, [text, compareTo, readable, fontSize]);
  return <div ref={host} data-testid="version-text" className="min-h-0 flex-1 overflow-auto" />;
}
