// Spike 1 (docs/plans/0001-m0-foundations.md, Phase 5): CodeMirror 6 with
// live preview on a generated 10,000-line note, measured where it runs.
// ?spike=cm6 shows it in any browser (add &run to measure at once); with
// FROSTNOTES_SPIKE=cm6 the desktop app measures, prints the results as one
// JSON line and exits (src-tauri/src/spike.rs). IME input cannot be
// automated: check it by hand in this view. "editor" measures the app's
// own editor, every live-preview provider on, the same way
// (docs/plans/0005-m3-desktop-app.md, Phase 5's exit); "editor/math,rendered"
// leaves those groups of providers out, to find what costs time.
import type { EditorView } from "@codemirror/view";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect, useRef, useState } from "react";

import { editorExtensions } from "../editor/extensions";
import { BASIC } from "../editor/preview/basic";
import { CODE } from "../editor/preview/code";
import { buildInline, livePreview, type Provider } from "../editor/preview/framework";
import { GUIDES } from "../editor/preview/guides";
import { INLINE } from "../editor/preview/inline";
import { LISTS } from "../editor/preview/lists";
import { MATH } from "../editor/preview/math";
import { MEDIA } from "../editor/preview/media";
import { PROPERTIES } from "../editor/preview/properties";
import { RENDERED } from "../editor/preview/rendered";
import { noteExtensions } from "./editor";
import { generateNote } from "./generate";
import { check, frames, fullParse, mount, scrolling, typing } from "./measure";

/** GROUPS are live preview's providers by file, in PROVIDERS' order. */
const GROUPS: Record<string, readonly Provider[]> = {
  properties: PROPERTIES,
  basic: BASIC,
  inline: INLINE,
  code: CODE,
  lists: LISTS,
  guides: GUIDES,
  math: MATH,
  rendered: RENDERED,
  media: MEDIA,
};

/**
 * appEditor is the app's editor with its defaults: live preview,
 * Obsidian's preferences; without names groups of providers to leave out.
 */
function appEditor(without: readonly string[]) {
  const providers = Object.entries(GROUPS).flatMap(([group, ps]) => (without.includes(group) ? [] : ps));
  return editorExtensions({
    prefs: {
      // Live preview comes from extra, with the providers chosen.
      live: false,
      readable: true,
      fontSize: 16,
      spellcheck: true,
      autoPairBrackets: true,
      autoPairMarkdown: true,
      smartLists: true,
      indentWithTabs: true,
      tabSize: 4,
      lineNumbers: false,
      foldHeadings: true,
      foldIndent: true,
      indentGuides: true,
    },
    env: { resolve: () => null, command: () => {} },
    onBlur: () => {},
    extra: livePreview(providers),
  });
}

export const LINES = 10_000;
export const KEYSTROKES = 200;

type Report = Record<string, unknown>;

function environment(): Report {
  return {
    userAgent: navigator.userAgent,
    devicePixelRatio: window.devicePixelRatio,
    viewport: [window.innerWidth, window.innerHeight],
    tauri: isTauri(),
  };
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export interface SpikeProps {
  name: string;
  /** Measure as soon as the note has loaded. */
  auto: boolean;
  /** Hand the results to the shell, which prints them and exits. */
  report: boolean;
}

export function Spike({ name, auto, report }: SpikeProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const loaded = useRef<{ note: string; result: Report } | null>(null);
  const started = useRef(false);
  const [status, setStatus] = useState("Loading…");
  const [result, setResult] = useState<Report | null>(null);
  const [running, setRunning] = useState(false);

  const finish = async (r: Report) => {
    const line = JSON.stringify(r);
    console.log(line);
    setResult(r);
    if (report) await invoke("spike_report", { report: line });
  };

  const measure = async () => {
    const v = view.current;
    const base = loaded.current;
    if (!v || !base) return;
    setRunning(true);
    try {
      await sleep(500);
      setStatus(`Typing ${KEYSTROKES} characters…`);
      const keystroke = await typing(v, KEYSTROKES);
      setStatus("Scrolling the whole note…");
      const scroll = await scrolling(v);
      setStatus("Checking live preview…");
      const checks = await check(v, name.startsWith("editor") ? (x) => countHidden(x) : undefined);
      const fullParseMs = fullParse(base.note);
      setStatus("Done");
      await finish({ ...base.result, keystroke, scroll, checks, parse: { fullParseMs } });
    } catch (err) {
      setStatus(`Failed: ${String(err)}`);
      await finish({ spike: name, ok: false, error: String(err) });
    } finally {
      setRunning(false);
    }
  };

  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once, when the page loads.
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void (async () => {
      const [kind, out = ""] = name.split("/");
      const without = out.split(",").filter((g) => g !== "");
      if ((kind !== "cm6" && kind !== "editor") || without.some((g) => !(g in GROUPS))) {
        setStatus(`Unknown spike "${name}"`);
        if (report) await finish({ spike: name, ok: false, error: "unknown spike" });
        return;
      }
      // Animation frames run only while the window is shown.
      if (isTauri()) await getCurrentWindow().show();
      await frames(10);
      const parent = host.current;
      if (!parent) return;
      const g0 = performance.now();
      const note = generateNote(LINES);
      const generateMs = Math.round((performance.now() - g0) * 100) / 100;
      const load = await mount(parent, note, kind === "editor" ? appEditor(without) : noteExtensions());
      view.current = load.view;
      const result: Report = {
        spike: name,
        ok: true,
        env: environment(),
        doc: { lines: load.view.state.doc.lines, chars: note.length, exact: load.view.state.doc.toString() === note },
        load: { generateMs, createMs: load.createMs, firstPaintMs: load.firstPaintMs },
      };
      loaded.current = { note, result };
      setResult(result);
      setStatus(auto ? "Measuring…" : "Loaded; Run measures typing and scrolling");
      if (auto) await measure();
    })();
    return () => view.current?.destroy();
  }, []);

  return (
    <div className="flex h-full flex-col bg-window">
      <header className="flex h-10 shrink-0 items-center gap-3 border-b border-separator bg-toolbar px-3">
        <span className="text-toolbar-title">Spike: CodeMirror 6 live preview, {LINES.toLocaleString("en")} lines</span>
        <span role="status" className="min-w-0 flex-1 truncate text-secondary">
          {status}
        </span>
        <button
          type="button"
          disabled={running || !loaded.current}
          onClick={() => void measure()}
          className="rounded-md bg-accent px-3 py-1 text-accent-contrast disabled:opacity-40"
        >
          Run
        </button>
      </header>
      <div className="flex min-h-0 flex-1">
        <div ref={host} className="min-w-0 flex-1" />
        {result && (
          <pre className="w-80 shrink-0 overflow-auto border-l border-separator bg-sidebar p-3 text-[11px] leading-4 select-text">
            {JSON.stringify(result, null, 2)}
          </pre>
        )}
      </div>
    </div>
  );
}

// countHidden counts the app editor's hidden ranges (marks of class
// cm-pv-hide) where the note shows.
function countHidden(view: EditorView): number {
  let n = 0;
  for (const it = buildInline(view.state, view.visibleRanges).iter(); it.value; it.next()) {
    if (it.value.spec.class === "cm-pv-hide") n++;
  }
  return n;
}
