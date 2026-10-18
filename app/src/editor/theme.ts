// The editor's look (docs/specs/ui.md#type, #editor-and-reading-view):
// Obsidian's sizes in the app's tokens, the page at most 700 wide while
// Readable line length is on, centered, 32px from the top and sides with
// half the view below the last line.
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import type { Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

const MONO = 'ui-monospace, "DejaVu Sans Mono", monospace';

const highlight = HighlightStyle.define([
  { tag: t.heading, fontWeight: "600" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "700" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: t.link, color: "var(--accent)" },
  { tag: t.url, color: "var(--text-tertiary)" },
  { tag: t.monospace, fontFamily: MONO, fontSize: "0.875em" },
  { tag: t.quote, color: "var(--text-secondary)" },
  { tag: [t.processingInstruction, t.meta, t.contentSeparator], color: "var(--text-tertiary)" },
  // Tags and footnote labels; highlights.
  { tag: t.labelName, color: "var(--accent)" },
  { tag: t.special(t.content), backgroundColor: "var(--bg-highlight)" },
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
      ".cm-pv-code": { fontFamily: MONO, backgroundColor: "var(--bg-code)" },
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
      ".cm-fn-comment": { color: "var(--text-tertiary)" },
      // Live preview's widgets (docs/specs/ui.md#editor-and-reading-view).
      ".cm-pv-tag": {
        backgroundColor: "var(--bg-tag)",
        color: "var(--accent)",
        borderRadius: "1em",
        padding: "0.1em 0.5em",
        fontSize: "0.875em",
        cursor: "pointer",
      },
      ".cm-pv-inline-code": {
        fontFamily: MONO,
        fontSize: "0.875em",
        backgroundColor: "var(--bg-code)",
        borderRadius: "4px",
        padding: "0.1em 0.25em",
      },
      ".cm-pv-footnote": { color: "var(--accent)", fontSize: "0.75em", cursor: "pointer" },
      ".cm-line.cm-pv-code": { padding: "0 16px" },
      ".cm-line.cm-pv-code-first": {
        position: "relative",
        paddingTop: "12px",
        borderTopLeftRadius: "6px",
        borderTopRightRadius: "6px",
      },
      ".cm-line.cm-pv-code-last": {
        paddingBottom: "12px",
        borderBottomLeftRadius: "6px",
        borderBottomRightRadius: "6px",
      },
      ".cm-pv-code-header": {
        position: "absolute",
        top: "6px",
        right: "12px",
        display: "flex",
        alignItems: "center",
        gap: "8px",
      },
      ".cm-pv-code-lang": {
        fontFamily: "inherit",
        fontSize: "11px",
        lineHeight: "14px",
        color: "var(--text-tertiary)",
      },
      ".cm-pv-code-copy": {
        fontFamily: "inherit",
        fontSize: "11px",
        lineHeight: "14px",
        padding: "2px 6px",
        borderRadius: "4px",
        border: "1px solid var(--separator)",
        backgroundColor: "var(--bg-window)",
        color: "var(--text-secondary)",
        cursor: "pointer",
        opacity: "0",
      },
      ".cm-line.cm-pv-code:hover .cm-pv-code-copy, .cm-pv-code-copy:focus-visible": { opacity: "1" },
      ".cm-line.cm-pv-quote": { borderLeft: "2px solid var(--accent)", paddingLeft: "16px" },
      ".cm-pv-bullet": {
        display: "inline-block",
        width: "6px",
        height: "6px",
        borderRadius: "50%",
        backgroundColor: "var(--text-secondary)",
        verticalAlign: "0.2em",
        margin: "0 0.35em",
      },
      ".cm-pv-hr": {
        display: "inline-block",
        width: "100%",
        verticalAlign: "middle",
        borderTop: "2px solid var(--separator)",
      },
      ".cm-pv-task-done": { color: "var(--text-secondary)", textDecoration: "line-through" },
      ".cm-pv-math-block": { textAlign: "center", padding: "8px 0" },
      ".cm-pv-rendered": { cursor: "text", padding: "4px 0" },
      ".cm-pv-embed": { borderLeft: "2px solid var(--accent)", padding: "4px 24px" },
      ".cm-pv-media": { maxWidth: "100%", verticalAlign: "bottom" },
      // Completion, drawn as the app's menus are.
      ".cm-tooltip": {
        backgroundColor: "var(--bg-window)",
        border: "1px solid var(--separator)",
        borderRadius: "8px",
        boxShadow: "var(--popover-shadow)",
      },
      ".cm-tooltip.cm-tooltip-autocomplete > ul": { fontFamily: "inherit", padding: "4px", maxHeight: "18em" },
      ".cm-tooltip.cm-tooltip-autocomplete > ul > li": {
        borderRadius: "4px",
        padding: "4px 8px",
        fontSize: "13px",
        lineHeight: "20px",
      },
      ".cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]": {
        backgroundColor: "var(--accent)",
        color: "var(--accent-contrast)",
      },
      ".cm-completionDetail": { marginLeft: "12px", fontStyle: "normal", color: "var(--text-tertiary)" },
      ".cm-tooltip-autocomplete li[aria-selected] .cm-completionDetail": { color: "inherit" },
      ".cm-completionMatchedText": { textDecoration: "none", fontWeight: "600" },
    }),
  ];
}
