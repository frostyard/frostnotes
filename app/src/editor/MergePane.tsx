// The merge view (docs/design/app.md#history-and-merges; ADR-0022;
// docs/specs/ui.md#merge-view): @codemirror/merge's side-by-side view, the
// note's document at the left, editable and saving as the editor does,
// and another version at the right, read-only: a conflict version, a hub
// revision, any version from the history, or the text the editor held when
// notesd refused its base. Taking a chunk from the right is an ordinary
// edit of the document, so it saves with its base and Ctrl+Z undoes it.
import { history } from "@codemirror/commands";
import { MergeView } from "@codemirror/merge";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { useEffect, useRef, useState } from "react";

import { useClient } from "../data/session";
import type { MergeSide } from "../workspace/model";
import { editorKeymap } from "./keymap";
import { markdownSupport } from "./syntax/index";
import { useNoteDoc } from "./useNoteDoc";

export interface MergePaneProps {
  vaultId: number;
  path: string;
  other: MergeSide;
  /** Done: the merge is finished. */
  onDone: () => void;
}

/** OtherSide is the right side's text and what to call it, or why it is missing. */
type OtherSide = { text: string; label: string } | { error: string };

const mergeTheme = EditorView.theme({
  "&": { color: "var(--text-primary)", backgroundColor: "var(--bg-window)", fontSize: "15px" },
  ".cm-scroller": { fontFamily: "inherit", lineHeight: "1.5" },
  ".cm-content": { padding: "16px 12px", caretColor: "var(--accent)" },
  ".cm-gutters": { backgroundColor: "transparent", border: "none" },
  "&.cm-merge-a .cm-changedLine, .cm-merge-a .cm-changedLine": {
    backgroundColor: "color-mix(in srgb, var(--flag-4) 15%, transparent)",
  },
  "&.cm-merge-b .cm-changedLine, .cm-merge-b .cm-changedLine": {
    backgroundColor: "color-mix(in srgb, var(--flag-1) 15%, transparent)",
  },
  ".cm-changedText": { background: "none", textDecoration: "underline" },
});

/** MergePane compares a note's document with another version of it. */
export function MergePane({ vaultId, path, other, onDone }: MergePaneProps) {
  const current = useClient();
  const client = useRef(current);
  client.current = current;
  const { doc, ready } = useNoteDoc(vaultId, path);
  const [side, setSide] = useState<OtherSide | null>(null);

  // The right side's text.
  const otherKey = JSON.stringify(other);
  useEffect(() => {
    if (!ready || !doc) return;
    let stale = false;
    const c = client.current;
    const load = async (): Promise<OtherSide> => {
      const o = JSON.parse(otherKey) as MergeSide;
      switch (o.kind) {
        case "version": {
          const v = await c.note.readVersion({ vaultId, version: o.version });
          const when = new Date(v.version.at).toLocaleString();
          const who = v.version.author || v.version.source;
          return {
            text: v.content,
            label: `${v.version.conflict ? "the conflict version" : "the version"} of ${when} by ${who}`,
          };
        }
        case "revision": {
          const r = await c.sync.readRevision({ vaultId, seq: o.seq });
          return { text: r.content, label: `${r.revision.deviceName}'s revision ${r.revision.seq}` };
        }
        case "held": {
          const n = doc.notice;
          return n?.kind === "refused"
            ? { text: n.text, label: "the text that was not saved" }
            : { error: "The text that was not saved is gone: the window keeps it only while it stays open." };
        }
      }
    };
    load()
      .then((s) => !stale && setSide(s))
      .catch((err: unknown) => !stale && setSide({ error: err instanceof Error ? err.message : String(err) }));
    return () => {
      stale = true;
    };
  }, [ready, doc, vaultId, otherKey]);

  // The two editors: the left one is a view of the document.
  const host = useRef<HTMLDivElement>(null);
  const text = side && "text" in side ? side.text : null;
  useEffect(() => {
    if (!doc || text === null || !host.current) return;
    let mv: MergeView | null = null;
    mv = new MergeView({
      a: {
        doc: doc.text,
        extensions: [
          markdownSupport(),
          history(),
          keymap.of(editorKeymap()),
          EditorView.lineWrapping,
          mergeTheme,
          EditorView.updateListener.of((u) => {
            if (mv) for (const tr of u.transactions) doc.local(tr, mv.a);
          }),
        ],
      },
      b: {
        doc: text,
        extensions: [
          markdownSupport(),
          EditorView.lineWrapping,
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          mergeTheme,
        ],
      },
      parent: host.current,
      revertControls: "b-to-a",
      highlightChanges: true,
      gutter: true,
    });
    const a = mv.a;
    doc.attach(a);
    return () => {
      doc.detach(a);
      mv?.destroy();
    };
  }, [doc, text]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="text-tab flex h-9 shrink-0 items-center gap-3 border-b border-separator px-3">
        <span className="min-w-0 flex-1 truncate">
          {side === null ? "Loading…" : "label" in side ? `Comparing with ${side.label}` : side.error}
        </span>
        <button type="button" className="text-accent hover:underline" onClick={onDone}>
          Done
        </button>
      </div>
      <div ref={host} data-testid="merge-view" className="min-h-0 flex-1 overflow-auto" />
    </div>
  );
}
