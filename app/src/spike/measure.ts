// Spike 1's measurements (docs/plans/0001-m0-foundations.md, Phase 5): load
// time, keystroke-to-paint and scroll frame times of the editor on a large
// note, plus checks that live preview drew and left the text alone.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState, type Extension } from "@codemirror/state";
import { Decoration, EditorView } from "@codemirror/view";

import { markdownSupport } from "./editor";
import { livePreview } from "./livePreview";

/** Stats summarize samples in milliseconds. */
export interface Stats {
  n: number;
  median: number;
  p95: number;
  max: number;
  mean: number;
}

const round = (v: number) => Math.round(v * 100) / 100;

/** percentile is the nearest-rank percentile p (0–100) of ascending sorted. */
export function percentile(sorted: readonly number[], p: number): number {
  if (sorted.length === 0) return Number.NaN;
  const rank = Math.ceil((p / 100) * sorted.length);
  return sorted[Math.min(sorted.length, Math.max(1, rank)) - 1] ?? Number.NaN;
}

export function stats(samples: readonly number[]): Stats {
  const sorted = [...samples].sort((a, b) => a - b);
  const sum = sorted.reduce((s, v) => s + v, 0);
  return {
    n: sorted.length,
    median: round(percentile(sorted, 50)),
    p95: round(percentile(sorted, 95)),
    max: round(sorted.at(-1) ?? Number.NaN),
    mean: round(sorted.length ? sum / sorted.length : Number.NaN),
  };
}

/** Frame is when the next animation frame started and when its rendering was done. */
interface Frame {
  start: number;
  painted: number;
}

/**
 * nextFrame resolves after the next frame. A message posted from the frame's
 * first animation callback runs once the frame's other callbacks, style,
 * layout and paint are done, which marks the paint. Call it before the work
 * whose frame it measures, so its callback runs first.
 */
export function nextFrame(): Promise<Frame> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => {
      const start = performance.now();
      const channel = new MessageChannel();
      channel.port1.onmessage = () => {
        channel.port1.close();
        resolve({ start, painted: performance.now() });
      };
      channel.port2.postMessage(null);
    });
  });
}

export async function frames(n: number): Promise<void> {
  for (let i = 0; i < n; i++) await nextFrame();
}

export interface LoadResult {
  view: EditorView;
  /** Creating the EditorView: state, first parse slice, DOM. */
  createMs: number;
  /** From creating the view to the end of its first painted frame. */
  firstPaintMs: number;
}

/** mount creates the editor on doc in parent and times it to its first paint. */
export async function mount(parent: HTMLElement, doc: string, extensions: Extension): Promise<LoadResult> {
  const t0 = performance.now();
  const view = new EditorView({ state: EditorState.create({ doc, extensions }), parent });
  const t1 = performance.now();
  const frame = await nextFrame();
  return { view, createMs: round(t1 - t0), firstPaintMs: round(frame.painted - t0) };
}

export interface KeystrokeResult {
  /**
   * The main thread's cost of a keystroke until it is on screen: dispatch
   * plus the next frame's work. Under one frame (16.7 ms at 60 Hz), every
   * keystroke shows in the next frame.
   */
  keystrokeToPaint: Stats;
  /** The transaction and its synchronous DOM update. */
  dispatch: Stats;
  /** The next frame: CodeMirror's measure, style, layout and paint. */
  frame: Stats;
  /** From dispatch to the end of the next painted frame, including the wait for it. */
  wall: Stats;
  /** The text after typing is the original with the typed characters inserted, and nothing else. */
  textExact: boolean;
}

const TYPED = " the quick brown fox jumps over the lazy dog, then the lantern and the wren";

/**
 * typing types n characters one transaction at a time at the end of a
 * paragraph in the middle of the note, as input.type, and times each.
 */
export async function typing(view: EditorView, n = 200): Promise<KeystrokeResult> {
  const doc = view.state.doc;
  let lineNo = Math.floor(doc.lines / 2);
  while (lineNo < doc.lines && !/^[A-Z]/.test(doc.line(lineNo).text)) lineNo++;
  const at = doc.line(lineNo).to;
  const before = doc.toString();
  view.focus();
  view.dispatch({ selection: { anchor: at }, effects: EditorView.scrollIntoView(at, { y: "center" }) });
  await frames(10);

  const dispatch: number[] = [];
  const frame: number[] = [];
  const total: number[] = [];
  const wall: number[] = [];
  let typed = "";
  for (let i = 0; i < n; i++) {
    const ch = TYPED[i % TYPED.length] ?? "x";
    const next = nextFrame();
    const head = view.state.selection.main.head;
    const t0 = performance.now();
    view.dispatch({
      changes: { from: head, insert: ch },
      selection: { anchor: head + 1 },
      userEvent: "input.type",
      scrollIntoView: true,
    });
    const t1 = performance.now();
    const f = await next;
    typed += ch;
    dispatch.push(t1 - t0);
    frame.push(f.painted - f.start);
    total.push(t1 - t0 + (f.painted - f.start));
    wall.push(f.painted - t0);
  }
  const textExact = view.state.doc.toString() === before.slice(0, at) + typed + before.slice(at);
  return {
    keystrokeToPaint: stats(total),
    dispatch: stats(dispatch),
    frame: stats(frame),
    wall: stats(wall),
    textExact,
  };
}

export interface ScrollResult {
  frames: number;
  distancePx: number;
  durationMs: number;
  fps: number;
  /** Intervals between animation frames while scrolling. */
  frameMs: Stats;
  /** Frames that took longer than one 60 Hz frame (with 10% slack), and than two. */
  over16ms: number;
  over33ms: number;
}

/**
 * scrolling scrolls the whole note top to bottom, step pixels per animation
 * frame, and times the frames.
 */
export async function scrolling(view: EditorView, step = 300, maxFrames = 20_000): Promise<ScrollResult> {
  const scroller = view.scrollDOM;
  scroller.scrollTop = 0;
  await frames(10);
  const times: number[] = [];
  await new Promise<void>((resolve) => {
    let stuck = 0;
    const tick = (now: number) => {
      times.push(now);
      const before = scroller.scrollTop;
      scroller.scrollTop = before + step;
      stuck = scroller.scrollTop <= before ? stuck + 1 : 0;
      if (stuck >= 3 || times.length >= maxFrames) resolve();
      else requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const intervals = times.slice(1).map((t, i) => t - (times[i] ?? t));
  const duration = (times.at(-1) ?? 0) - (times[0] ?? 0);
  return {
    frames: intervals.length,
    distancePx: Math.round(scroller.scrollTop),
    durationMs: round(duration),
    fps: round(duration > 0 ? (intervals.length * 1000) / duration : 0),
    frameMs: stats(intervals),
    over16ms: intervals.filter((v) => v > 16.7 * 1.1).length,
    over33ms: intervals.filter((v) => v > 33.4 * 1.1).length,
  };
}

export interface Checks {
  /** CodeMirror's own styles applied (the CSP let them in). */
  stylesApplied: boolean;
  /** Live preview's hidden ranges and checkboxes in the visible part of the note's top. */
  hiddenRanges: number;
  checkboxes: number;
  /** Clicking a checkbox changed exactly one character, and clicking it again restored the text. */
  toggleExact: boolean;
}

/** check verifies live preview at the top of the note. */
export async function check(view: EditorView): Promise<Checks> {
  view.dispatch({ selection: { anchor: 0 }, effects: EditorView.scrollIntoView(0, { y: "start" }) });
  await frames(5);
  let hiddenRanges = 0;
  const decorations = view.plugin(livePreview)?.decorations ?? Decoration.none;
  for (const it = decorations.iter(); it.value; it.next()) {
    if (it.value.spec.preview === "hide") hiddenRanges++;
  }
  const checkboxes = view.contentDOM.querySelectorAll("input.cm-pv-task").length;
  const before = view.state.doc.toString();
  const click = () => {
    const box = view.contentDOM.querySelector("input.cm-pv-task");
    box?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true, button: 0 }));
  };
  click();
  const once = view.state.doc.toString();
  click();
  return {
    stylesApplied: getComputedStyle(view.dom).display === "flex",
    hiddenRanges,
    checkboxes,
    toggleExact: differsByOne(before, once) && view.state.doc.toString() === before,
  };
}

// differsByOne reports whether a and b have the same length and differ in
// exactly one character.
function differsByOne(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let n = 0;
  for (let i = 0; i < a.length && n < 2; i++) if (a.charCodeAt(i) !== b.charCodeAt(i)) n++;
  return n === 1;
}

/** fullParse times parsing the whole document from scratch, off screen. */
export function fullParse(doc: string): number {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  const t0 = performance.now();
  ensureSyntaxTree(state, state.doc.length, 60_000);
  return round(performance.now() - t0);
}
