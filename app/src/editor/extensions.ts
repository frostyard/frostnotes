// The editor's extensions (docs/design/app.md#extensions): the parser, the
// keymap, history, search, link and tag completion, live preview (or
// source mode, without it), and the preferences that shape typing. Compartments let a view switch modes
// and follow the preferences without being made again.
import { autocompletion, closeBrackets } from "@codemirror/autocomplete";
import { history } from "@codemirror/commands";
import { markdownKeymap } from "@codemirror/lang-markdown";
import { codeFolding, foldGutter, indentUnit } from "@codemirror/language";
import { search } from "@codemirror/search";
import { Compartment, EditorState, type Extension, Prec } from "@codemirror/state";
import { drawSelection, EditorView, keymap, lineNumbers } from "@codemirror/view";

import { type CompletionCatalog, linkCompletion, tagCompletion } from "./complete";
import { editorKeymap } from "./keymap";
import { livePreview, type PreviewEnvInput, previewEnv } from "./preview/framework";
import { PROVIDERS } from "./preview/index";
import { propertiesState } from "./preview/properties";
import { markdownSupport } from "./syntax/index";
import { editorTheme } from "./theme";
import { titleCompartment } from "./title";
import { indentFolding, markdownPairs } from "./typing";

/** EditorPrefs are the preferences the editor follows (docs/specs/ui.md#settings-batch-2). */
export interface EditorPrefs {
  live: boolean;
  readable: boolean;
  fontSize: number;
  spellcheck: boolean;
  autoPairBrackets: boolean;
  autoPairMarkdown: boolean;
  smartLists: boolean;
  indentWithTabs: boolean;
  tabSize: number;
  lineNumbers: boolean;
  foldHeadings: boolean;
  foldIndent: boolean;
  indentGuides: boolean;
}

/** The compartments a view reconfigures. */
export const modeCompartment = new Compartment();
export const prefsCompartment = new Compartment();

/** modeExtension is live preview, or nothing for source mode. */
export function modeExtension(live: boolean): Extension {
  return live ? livePreview(PROVIDERS) : [];
}

/** prefsExtension is what the preferences set. */
export function prefsExtension(p: EditorPrefs): Extension {
  return [
    editorTheme({ readable: p.readable, fontSize: p.fontSize }),
    EditorView.contentAttributes.of({ spellcheck: p.spellcheck ? "true" : "false" }),
    p.autoPairBrackets ? closeBrackets() : [],
    p.autoPairMarkdown ? markdownPairs : [],
    // Smart lists: Enter continues a list or quote, Backspace takes its mark back.
    p.smartLists ? Prec.high(keymap.of(markdownKeymap)) : [],
    EditorState.tabSize.of(p.tabSize),
    indentUnit.of(p.indentWithTabs ? "\t" : " ".repeat(p.tabSize)),
    p.lineNumbers ? lineNumbers() : [],
    // Headings fold with the markdown language; Fold indent adds folding by indentation.
    p.foldHeadings || p.foldIndent ? [codeFolding(), foldGutter()] : [],
    p.foldIndent ? indentFolding : [],
    p.indentGuides ? [] : EditorView.editorAttributes.of({ class: "cm-no-guides" }),
  ];
}

/** editorExtensions are a view's extensions; onBlur saves; catalog, when given, completes links and tags; title is the inline title. */
export function editorExtensions(opts: {
  prefs: EditorPrefs;
  env: PreviewEnvInput;
  onBlur: () => void;
  catalog?: CompletionCatalog;
  /** The inline title (title.tsx), when the view shows one. */
  title?: Extension;
  /** More extensions: the note editor's page previews. */
  extra?: Extension;
}): Extension {
  return [
    markdownSupport(),
    history(),
    drawSelection(),
    EditorView.lineWrapping,
    keymap.of(editorKeymap()),
    search({ top: true }),
    opts.catalog
      ? autocompletion({ override: [linkCompletion(opts.catalog), tagCompletion(opts.catalog)], icons: false })
      : [],
    previewEnv.of(opts.env),
    propertiesState,
    titleCompartment.of(opts.title ?? []),
    opts.extra ?? [],
    modeCompartment.of(modeExtension(opts.prefs.live)),
    prefsCompartment.of(prefsExtension(opts.prefs)),
    EditorView.domEventHandlers({
      blur: () => {
        opts.onBlur();
        return false;
      },
    }),
  ];
}
