// The spike's editor: CodeMirror 6 over the note's markdown (GFM plus
// wikilinks), with live preview, styled with the app's tokens (ADR-0007,
// ADR-0011).
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import type { Extension } from "@codemirror/state";
import { drawSelection, EditorView, keymap } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

import { livePreview } from "./livePreview";
import { WikiLink } from "./wikilink";

/** markdownSupport parses GFM (markdownLanguage) with wikilinks. */
export function markdownSupport(): Extension {
  return markdown({ base: markdownLanguage, extensions: [WikiLink] });
}

const highlight = HighlightStyle.define([
  { tag: t.heading, fontWeight: "650" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "650" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: t.link, color: "var(--accent)" },
  { tag: t.url, color: "var(--text-tertiary)" },
  { tag: t.monospace, fontFamily: "ui-monospace, monospace", fontSize: "0.92em" },
  { tag: t.quote, color: "var(--text-secondary)" },
  { tag: [t.processingInstruction, t.meta, t.contentSeparator], color: "var(--text-tertiary)" },
]);

const theme = EditorView.theme({
  "&": { height: "100%", color: "var(--text-primary)", backgroundColor: "var(--bg-window)" },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": { fontFamily: "var(--font-sans)", fontSize: "15px", lineHeight: "1.6" },
  ".cm-content": {
    maxWidth: "46rem",
    margin: "0 auto",
    padding: "32px 24px 50vh",
    userSelect: "text",
    cursor: "text",
    caretColor: "var(--accent)",
  },
  ".cm-pv-h1": { fontSize: "1.6em", fontWeight: "700", lineHeight: "1.4" },
  ".cm-pv-h2": { fontSize: "1.35em", fontWeight: "650", lineHeight: "1.4" },
  ".cm-pv-h3": { fontSize: "1.2em", fontWeight: "650" },
  ".cm-pv-h4, .cm-pv-h5, .cm-pv-h6": { fontWeight: "650" },
  ".cm-pv-link, .cm-pv-wikilink": { color: "var(--accent)", textDecoration: "underline", cursor: "pointer" },
  ".cm-pv-code": {
    fontFamily: "ui-monospace, monospace",
    fontSize: "0.92em",
    backgroundColor: "var(--bg-sidebar)",
  },
  ".cm-pv-task": { margin: "0 0.45em 0 0", verticalAlign: "-0.1em", accentColor: "var(--accent)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": { backgroundColor: "var(--selection-inactive)" },
});

/** noteExtensions are the editor's extensions; source mode is the same without livePreview. */
export function noteExtensions(): Extension[] {
  return [
    markdownSupport(),
    syntaxHighlighting(highlight),
    livePreview,
    history(),
    drawSelection(),
    keymap.of([...defaultKeymap, ...historyKeymap]),
    EditorView.lineWrapping,
    theme,
  ];
}
