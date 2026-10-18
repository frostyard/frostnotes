// The editor's look (docs/specs/ui.md#type, #editor-and-reading-view):
// Obsidian's sizes in the app's tokens, the page at most 700 wide while
// Readable line length is on, centered, 32px from the top and sides with
// half the view below the last line.
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import type { Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

const highlight = HighlightStyle.define([
  { tag: t.heading, fontWeight: "600" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "700" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: t.link, color: "var(--accent)" },
  { tag: t.url, color: "var(--text-tertiary)" },
  { tag: t.monospace, fontFamily: 'ui-monospace, "DejaVu Sans Mono", monospace', fontSize: "0.875em" },
  { tag: t.quote, color: "var(--text-secondary)" },
  { tag: [t.processingInstruction, t.meta, t.contentSeparator], color: "var(--text-tertiary)" },
]);

/** editorTheme is the editor's styles; readable caps the page at 700px, fontSize is the body's. */
export function editorTheme(opts: { readable: boolean; fontSize: number }): Extension {
  return [
    syntaxHighlighting(highlight),
    EditorView.theme({
      "&": { height: "100%", color: "var(--text-primary)", backgroundColor: "var(--bg-window)" },
      "&.cm-focused": { outline: "none" },
      ".cm-scroller": { fontFamily: "inherit", fontSize: `${opts.fontSize}px`, lineHeight: "1.5" },
      ".cm-content": {
        maxWidth: opts.readable ? "700px" : "none",
        margin: "0 auto",
        padding: "32px 32px 50vh",
        caretColor: "var(--accent)",
      },
      ".cm-cursor": { borderLeftColor: "var(--accent)" },
      ".cm-pv-h1": { fontSize: "1.618em", fontWeight: "700", lineHeight: "1.2" },
      ".cm-pv-h2": { fontSize: "1.462em", fontWeight: "600", lineHeight: "1.2" },
      ".cm-pv-h3": { fontSize: "1.318em", fontWeight: "600", lineHeight: "1.3" },
      ".cm-pv-h4": { fontSize: "1.188em", fontWeight: "600", lineHeight: "1.4" },
      ".cm-pv-h5": { fontSize: "1.076em", fontWeight: "600" },
      ".cm-pv-h6": { fontSize: "1em", fontWeight: "600" },
      ".cm-pv-link": { color: "var(--accent)", textDecoration: "underline", cursor: "pointer" },
      ".cm-pv-unresolved": { opacity: "0.7" },
      ".cm-pv-code": { fontFamily: 'ui-monospace, "DejaVu Sans Mono", monospace', backgroundColor: "var(--bg-code)" },
      ".cm-pv-task": {
        width: "16px",
        height: "16px",
        margin: "0 0.45em 0 0",
        verticalAlign: "-0.15em",
        accentColor: "var(--accent)",
      },
      ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": {
        backgroundColor: "var(--selection-inactive)",
      },
      ".cm-panels": { backgroundColor: "var(--bg-toolbar)", color: "var(--text-primary)" },
      ".cm-panels.cm-panels-top": { borderBottom: "1px solid var(--separator)" },
      ".cm-searchMatch": { backgroundColor: "var(--bg-highlight)" },
    }),
  ];
}
