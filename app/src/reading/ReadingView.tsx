// The reading view (docs/design/app.md#reading-view; ADR-0007): notesd
// renders the note's document (its text, saved or not) with note.render;
// the app sanitizes the HTML again, gives each data-blob element its blob
// URL, draws math with KaTeX from data-tex alone, and puts the result in
// the view. Links and embeds follow as the editor's do; a checkbox
// toggles its line in the document, which saves as typing does; a tag
// searches for itself; a foldable callout's title folds it.
import { type MouseEvent, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import { usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import type { NoteDoc, Notice } from "../editor/document";
import { NoticeBar, titleOf } from "../editor/NoteEditor";
import type { PreviewCommand } from "../editor/preview/framework";
import { useNoteDoc } from "../editor/useNoteDoc";
import { InlineTitle } from "../features/workspace/InlineTitle";
import { type OpenWhere, whereFor } from "../workspace/ops";
import { finish } from "./finish";
import { createSanitizer } from "./sanitize";

/** RENDER_DELAY is how long after a change the view renders again. */
export const RENDER_DELAY = 300;

export interface ReadingViewProps {
  vaultId: number;
  path: string;
  onFollow: (c: PreviewCommand) => void;
  onCompare: (notice: Notice) => void;
  onClose: () => void;
  /** Rename the note from its inline title. */
  onRename?: (title: string) => void;
}

/** ReadingView shows a note as notesd renders it. */
export function ReadingView(props: ReadingViewProps) {
  const { vaultId, path } = props;
  const current = useClient();
  const client = useRef(current);
  client.current = current;
  const { doc, snap, ready } = useNoteDoc(vaultId, path);
  const sanitize = useMemo(() => createSanitizer(window), []);
  const [html, setHtml] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Render now, and again a moment after each change of the text.
  useEffect(() => {
    if (!ready || !doc) return;
    let stale = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const render = async () => {
      try {
        const r = await client.current.note.render({ vaultId, path: doc.path, content: doc.text.toString() });
        if (!stale) {
          setHtml(sanitize(r.html));
          setError(null);
        }
      } catch (err) {
        if (!stale) setError(err instanceof Error ? err.message : String(err));
      }
    };
    void render();
    const off = doc.onText(() => {
      clearTimeout(timer);
      timer = setTimeout(() => void render(), RENDER_DELAY);
    });
    return () => {
      stale = true;
      clearTimeout(timer);
      off();
    };
  }, [ready, doc, vaultId, sanitize]);

  const readable = usePrefs((s) => s.readableLineLength);
  const showTitle = usePrefs((s) => s.inlineTitle);
  const remoteImages = usePrefs((s) => s.loadRemoteImages);
  const box = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el || html === null) return;
    el.innerHTML = html;
    finish(el, (hash, ext) => client.current.transport.blobURL?.(hash, ext) ?? "", remoteImages);
  }, [html, remoteImages]);

  const onClick = (e: MouseEvent<HTMLDivElement>) => {
    if (doc) click(e, doc, props.onFollow);
  };
  return (
    <div className="flex h-full min-h-0 flex-col">
      {doc && snap.notice && <NoticeBar notice={snap.notice} doc={doc} props={props} />}
      {error && (
        <p className="text-empty px-8 py-4 text-secondary">
          Cannot render {path}: {error}
        </p>
      )}
      {snap.status === "missing" && <p className="text-empty px-8 py-8 text-secondary">{path} is not in the vault.</p>}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {showTitle && (
          <div className={`mx-auto px-8 pt-8 ${readable ? "max-w-[764px]" : ""}`}>
            <InlineTitle title={titleOf(snap.path)} onRename={(t) => props.onRename?.(t)} />
          </div>
        )}
        {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noStaticElementInteractions: the clicks are on the links and checkboxes inside, which keep their own roles and keys */}
        <div
          ref={box}
          data-testid="reading-view"
          className={`reading-view rendered mx-auto px-8 ${showTitle ? "" : "pt-8"} pb-[50vh] ${readable ? "max-w-[764px]" : ""}`}
          onClick={onClick}
          onAuxClick={onClick}
        />
      </div>
    </div>
  );
}

// click follows a link or toggles a checkbox the reading view drew.
function click(e: MouseEvent<HTMLElement>, doc: NoteDoc, onFollow: (c: PreviewCommand) => void): void {
  const target = e.target instanceof Element ? e.target : null;
  if (!target) return;
  const where: OpenWhere = whereFor(e);
  // A foldable callout's title folds it, here only, as Obsidian does.
  const title = target.closest<HTMLElement>(".callout[data-fold] > .callout-title");
  if (title?.parentElement) {
    const callout = title.parentElement;
    callout.dataset.fold = callout.dataset.fold === "-" ? "+" : "-";
    return;
  }
  const box = target.closest<HTMLInputElement>('input[type="checkbox"][data-line]');
  if (box) {
    e.preventDefault();
    toggleLine(doc, Number(box.dataset.line));
    return;
  }
  const tag = target.closest<HTMLAnchorElement>("a.tag[data-tag]");
  if (tag) {
    e.preventDefault();
    onFollow({ kind: "search", query: `tag:#${tag.dataset.tag ?? ""}` });
    return;
  }
  const link = target.closest<HTMLAnchorElement>("a.internal-link, a.file-embed, a.external-link");
  if (!link) return;
  e.preventDefault();
  if (link.classList.contains("external-link")) {
    onFollow({ kind: "follow", target: link.getAttribute("href") ?? "", path: null, where });
    return;
  }
  const href = (link.dataset.href ?? "").split("#")[0] ?? "";
  onFollow({ kind: "follow", target: href, path: link.dataset.path ?? null, where });
}

/** toggleLine toggles the task on a note's line (1-based): its marker's character becomes "x" or " ". */
export function toggleLine(doc: NoteDoc, line: number): void {
  if (!Number.isInteger(line) || line < 1 || line > doc.text.lines) return;
  const l = doc.text.line(line);
  const m = /^(\s*(?:[-*+]|\d+[.)])\s+\[)(.)\]/.exec(l.text);
  if (!m) return;
  const at = l.from + (m[1]?.length ?? 0);
  doc.edit({ from: at, to: at + 1, insert: m[2] === " " ? "x" : " " });
}
