// A note's editor in a tab (docs/design/app.md#editor): a view of the
// note's document, in live preview or source mode, with the document's
// notice above it. The view saves when it loses focus, and its document
// saves when the last view of it goes.
import { EditorView } from "@codemirror/view";
import { useEffect, useMemo, useRef } from "react";

import { useFiles } from "../data/files";
import { openPreview } from "../data/overlays";
import { type Prefs, usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { TabNotice, type TabNoticeAction } from "../features/workspace/TabNotice";
import { extensionOf } from "../lib/fileTree";
import { fill } from "../reading/finish";
import { createSanitizer } from "../reading/sanitize";
import { completionCatalog } from "./catalog";
import type { NoteDoc, Notice } from "./document";
import {
  type EditorPrefs,
  editorExtensions,
  modeCompartment,
  modeExtension,
  prefsCompartment,
  prefsExtension,
} from "./extensions";
import { LinkResolver } from "./links";
import { type PreviewCommand, redraw } from "./preview/framework";
import { useDocuments } from "./registry";
import { BlockRenderer } from "./rendered";
import { frontmatterEnd } from "./syntax/extract";
import { inlineTitle, titleCompartment } from "./title";
import { useNoteDoc } from "./useNoteDoc";

export interface NoteEditorProps {
  tabId: string;
  vaultId: number;
  path: string;
  /** Live preview, else source mode. */
  live: boolean;
  /** Where the cursor starts. */
  cursor?: number;
  /** Take focus when the editor appears: its tab is the active one. */
  autoFocus: boolean;
  /** A link was followed. */
  onCommand: (c: PreviewCommand) => void;
  /** The notice's Compare: open the merge view. */
  onCompare: (notice: Notice) => void;
  /** Close the tab (a deleted note's notice). */
  onClose: () => void;
  /** Keep where the cursor was in the note at path, for the tab. */
  onCursor?: (pos: number, path: string) => void;
  /** Rename the note from its inline title. */
  onRename?: (title: string) => void;
}

/** titleOf is a note's title: its file name without ".md". */
export function titleOf(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1).replace(/\.md$/i, "");
}

/** REFRESH_DELAY is how long after the vault changes rendered blocks render again, in ms. */
export const REFRESH_DELAY = 1000;

function editorPrefs(p: Prefs, live: boolean): EditorPrefs {
  return {
    live,
    readable: p.readableLineLength,
    fontSize: p.fontSize,
    spellcheck: p.spellcheck,
    autoPairBrackets: p.autoPairBrackets,
    autoPairMarkdown: p.autoPairMarkdown,
    smartLists: p.smartLists,
    indentWithTabs: p.indentWithTabs,
    tabSize: p.tabSize,
    lineNumbers: p.lineNumbers,
    foldHeadings: p.foldHeadings,
    foldIndent: p.foldIndent,
    indentGuides: p.indentGuides,
  };
}

/** NoteEditor edits a note in a tab. */
export function NoteEditor(props: NoteEditorProps) {
  const { vaultId, path } = props;
  const registry = useDocuments();
  // The client changes on a reconnect; the view and its resolver stay.
  const current = useClient();
  const client = useRef(current);
  client.current = current;
  const { doc, snap, ready } = useNoteDoc(vaultId, path);

  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const latest = useRef(props);
  latest.current = props;
  const prefs = usePrefs();
  const ep = editorPrefs(prefs, props.live);
  const sanitize = useMemo(() => createSanitizer(window), []);
  const renderer = useMemo(
    () =>
      doc &&
      new BlockRenderer(
        async (markdown) =>
          sanitize((await client.current.note.render({ vaultId, path: doc.path, content: markdown })).html),
        (el, html) =>
          fill(
            el,
            html,
            (hash, ext) => client.current.transport.blobURL?.(hash, ext) ?? "",
            usePrefs.getState().loadRemoteImages,
          ),
        () => view.current?.requestMeasure(),
      ),
    [doc, vaultId, sanitize],
  );
  // The vault's property types, for the properties block, read with note.properties.
  const types = useRef<Readonly<Record<string, string>>>({});
  const rename = useMemo(() => (title: string) => latest.current.onRename?.(title), []);
  const resolver = useMemo(
    () =>
      doc &&
      new LinkResolver(
        (targets) => client.current.note.resolve({ vaultId, from: doc.path, targets }),
        () => view.current?.dispatch({ effects: redraw.of(null) }),
      ),
    [doc, vaultId],
  );

  // The view: made once the document has loaded, gone with the tab.
  useEffect(() => {
    if (!ready || !doc || !resolver || !renderer || !host.current) return;
    const start = latest.current;
    const v = new EditorView({
      parent: host.current,
      state: doc.createState(
        editorExtensions({
          prefs: editorPrefs(usePrefs.getState(), start.live),
          env: {
            resolve: resolver.resolve,
            command: (c) => latest.current.onCommand(c),
            fileURL: (p) => {
              const file = useFiles.getState().files.get(p);
              return file ? (client.current.transport.blobURL?.(file.hash, extensionOf(p)) ?? null) : null;
            },
            render: renderer.fill,
            remoteImages: () => usePrefs.getState().loadRemoteImages,
            propertiesMode: () => usePrefs.getState().properties,
            propertyTypes: () => types.current,
          },
          title: inlineTitle(usePrefs.getState().inlineTitle, titleOf(doc.path), rename),
          // Ctrl over a link shows the note it leads to (its page preview).
          extra: EditorView.domEventHandlers({
            mousemove: (e) => {
              if (!e.ctrlKey || !(e.target instanceof Element)) return false;
              const el = e.target.closest<HTMLElement>(".cm-pv-link[data-target]");
              const target = el?.dataset.target ?? "";
              const to = target === "" ? undefined : resolver.resolve(target);
              if (!el || typeof to !== "string" || !to.endsWith(".md")) return false;
              const r = el.getBoundingClientRect();
              openPreview({
                anchor: { left: r.left, top: r.top, bottom: r.bottom },
                path: to,
                fill: (box) => renderer.fill(box, `![[${target}]]`),
              });
              return false;
            },
          }),
          onBlur: () => void doc.save(),
          catalog: completionCatalog({
            vaultId,
            path: () => doc.path,
            state: () => view.current?.state ?? null,
            client: () => client.current,
            docs: registry,
          }),
        }),
        // A note opens with the cursor where its body starts, after any frontmatter.
        { anchor: start.cursor ?? frontmatterEnd(doc.text.sliceString(0, 1 << 20)) },
      ),
      dispatchTransactions: (trs, ev) => {
        ev.update(trs);
        for (const tr of trs) doc.local(tr, ev);
        if (trs.some((tr) => tr.docChanged || tr.selection)) registry.editors.changed();
      },
    });
    view.current = v;
    doc.attach(v);
    const off = registry.editors.set(start.tabId, { view: v, doc });
    if (start.autoFocus) v.focus();
    return () => {
      latest.current.onCursor?.(v.state.selection.main.head, doc.path);
      off();
      doc.detach(v);
      v.destroy();
      view.current = null;
    };
  }, [ready, doc, resolver, renderer, registry, vaultId, rename]);

  // Modes and preferences reconfigure the view.
  useEffect(() => {
    view.current?.dispatch({ effects: modeCompartment.reconfigure(modeExtension(props.live)) });
  }, [props.live]);
  const prefsKey = JSON.stringify(ep);
  useEffect(() => {
    view.current?.dispatch({
      effects: prefsCompartment.reconfigure(prefsExtension(JSON.parse(prefsKey) as EditorPrefs)),
    });
  }, [prefsKey]);

  // Rendered blocks show the vault as it is a moment after it changes,
  // and remote images as the preference says; the properties block takes
  // the vault's property types as they change.
  const files = useFiles((s) => s.files);
  const remoteImages = usePrefs((s) => s.loadRemoteImages);
  // biome-ignore lint/correctness/useExhaustiveDependencies: files and remoteImages are the triggers
  useEffect(() => {
    if (!renderer) return;
    const timer = setTimeout(() => {
      renderer.refresh();
      view.current?.dispatch({ effects: redraw.of(null) });
      client.current.note
        .properties({ vaultId })
        .then((counts) => {
          types.current = Object.fromEntries(counts.map((c) => [c.key.toLowerCase(), c.type]));
          view.current?.dispatch({ effects: redraw.of(null) });
        })
        .catch(() => {});
    }, REFRESH_DELAY);
    return () => clearTimeout(timer);
  }, [renderer, files, remoteImages, vaultId]);

  // The frontmatter shows as the preference says; the inline title follows
  // its preference and the note's name.
  const propertiesMode = usePrefs((s) => s.properties);
  useEffect(() => {
    void propertiesMode;
    view.current?.dispatch({ effects: redraw.of(null) });
  }, [propertiesMode]);
  const showTitle = usePrefs((s) => s.inlineTitle);
  useEffect(() => {
    view.current?.dispatch({
      effects: titleCompartment.reconfigure(inlineTitle(showTitle, titleOf(snap.path), rename)),
    });
  }, [showTitle, snap.path, rename]);

  // A file added, moved or removed changes what links lead to.
  const pathsVersion = useFiles((s) => s.pathsVersion);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the vault's paths and the note's own are what make answers stale
  useEffect(() => {
    resolver?.reset();
  }, [resolver, pathsVersion, snap.path]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      {doc && snap.notice && <NoticeBar notice={snap.notice} doc={doc} props={props} />}
      {snap.status === "missing" && (
        <p className="text-empty mx-auto max-w-[700px] px-8 py-8 text-secondary">{path} is not in the vault.</p>
      )}
      <div ref={host} className="min-h-0 flex-1" data-testid="note-editor" />
    </div>
  );
}

/** NoticeBar draws a document's notice over a view of it. */
export function NoticeBar({
  notice,
  doc,
  props,
}: {
  notice: Notice;
  doc: NoteDoc;
  props: { onCompare: (notice: Notice) => void; onClose: () => void };
}) {
  let text: string;
  let actions: TabNoticeAction[];
  switch (notice.kind) {
    case "conflict":
      text = `${notice.conflicts} ${notice.conflicts === 1 ? "change" : "changes"} conflicted with ${notice.by}; your text is kept as a version.`;
      actions = [
        { label: "Compare", onClick: () => props.onCompare(notice) },
        { label: "Dismiss", onClick: () => doc.dismiss() },
      ];
      break;
    case "refused":
      text = "Not saved: notesd no longer has the version this text started from. Your text is kept.";
      actions = [{ label: "Compare", onClick: () => props.onCompare(notice) }];
      break;
    case "failed":
      text = `Not saved: ${notice.reason}. Retrying.`;
      actions = [{ label: "Retry now", onClick: () => doc.retryNow() }];
      break;
    case "deleted":
      text = "This note was deleted.";
      actions = [
        { label: "Restore", onClick: () => void doc.restore() },
        { label: "Close", onClick: props.onClose },
      ];
      break;
  }
  return <TabNotice text={text} actions={actions} />;
}
