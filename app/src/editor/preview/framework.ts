// The live-preview framework (docs/design/app.md#live-preview; ADR-0007):
// decoration over the source, never a transformation of it. A widget kind
// is a provider: the syntax nodes it draws and a function from a node and
// a context to decorations. Inline providers run over the visible ranges
// in a view plugin; block providers, whose widgets may replace line
// breaks, run in a state field over the whole document, again only around
// what changed. Syntax shows as typed on every line a selection touches;
// a block widget shows its source while a selection is inside it. A
// widget's clicks go to its provider, which answers with a transaction or
// a command.
import { syntaxTree } from "@codemirror/language";
import {
  type EditorState,
  type Extension,
  Facet,
  type Range,
  RangeSetBuilder,
  StateEffect,
  StateField,
  type Transaction,
  type TransactionSpec,
} from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView, ViewPlugin, type ViewUpdate } from "@codemirror/view";
import type { SyntaxNodeRef } from "@lezer/common";

import type { OpenWhere } from "../../workspace/ops";

/** PreviewCommand is what a provider asks of the app: follow a link to a file, or to a note to create. */
export type PreviewCommand = { kind: "follow"; target: string; path: string | null; where: OpenWhere };

/** PreviewAction is a provider's answer to a click. */
export type PreviewAction =
  | { kind: "transaction"; spec: TransactionSpec }
  | { kind: "command"; command: PreviewCommand };

/** Resolve gives the file a link target leads to: a path, null when unresolved, undefined while it is asked. */
export type Resolve = (target: string) => string | null | undefined;

/** PreviewEnv is what the app gives the preview: link resolution and where commands go. */
export interface PreviewEnv {
  resolve: Resolve;
  command: (c: PreviewCommand) => void;
}

/** previewEnv provides the app's PreviewEnv to the providers. */
export const previewEnv = Facet.define<PreviewEnv, PreviewEnv>({
  combine: (values) => values[0] ?? { resolve: () => null, command: () => {} },
});

/** DrawContext is what a provider draws with. */
export interface DrawContext {
  readonly state: EditorState;
  readonly env: PreviewEnv;
  /** The selection touches a line between from and to: its syntax shows as typed. */
  revealed(from: number, to?: number): boolean;
  /** A selection range overlaps from–to. */
  touches(from: number, to: number): boolean;
  /** add draws a decoration; an inline provider's may not cross a line break. */
  add(range: Range<Decoration>): void;
  /** hide hides from–to, on one line. */
  hide(from: number, to: number): void;
  /** line decorates the line at pos, once a line per decoration's class. */
  line(pos: number, deco: Decoration): void;
}

/** Provider is a kind of widget or styling. */
export interface Provider {
  /** Its name; elements it draws carry it as data-pv, so their clicks come back to it. */
  id: string;
  /** "inline" runs over what is visible; "block" over the whole document, and may replace line breaks. */
  kind: "inline" | "block";
  /** The syntax node names it draws. */
  nodes: readonly string[];
  /** draw draws a node; false skips the node's children. */
  draw(node: SyntaxNodeRef, cx: DrawContext): boolean | undefined;
  /** click answers a click on an element it drew. */
  click?(el: HTMLElement, view: EditorView, event: MouseEvent): PreviewAction | null;
}

/** previewProviders collects the providers; live preview is on while there are any. */
export const previewProviders = Facet.define<Provider>();

const hidden = Decoration.replace({});

interface Span {
  from: number;
  to: number;
}

// draw runs the providers of one kind over spans of state.
function draw(state: EditorState, spans: readonly Span[], providers: readonly Provider[]): Range<Decoration>[] {
  const byNode = new Map<string, Provider[]>();
  for (const p of providers) for (const n of p.nodes) byNode.set(n, [...(byNode.get(n) ?? []), p]);
  if (byNode.size === 0) return [];
  const doc = state.doc;
  const env = state.facet(previewEnv);
  const sel = state.selection.ranges.map((r) => ({ from: doc.lineAt(r.from).from, to: doc.lineAt(r.to).to }));
  const out: Range<Decoration>[] = [];
  const lines = new Set<string>();
  const seen = new Set<string>();
  const cx: DrawContext = {
    state,
    env,
    revealed: (from, to = from) => sel.some((r) => r.from <= to && from <= r.to),
    touches: (from, to) => state.selection.ranges.some((r) => r.from <= to && r.to >= from),
    add: (range) => out.push(range),
    hide: (from, to) => {
      if (from < to && doc.lineAt(from).number === doc.lineAt(to).number) out.push(hidden.range(from, to));
    },
    line: (pos, deco) => {
      const start = doc.lineAt(pos).from;
      const k = `${start}:${deco.spec.class}`;
      if (lines.has(k)) return;
      lines.add(k);
      out.push(deco.range(start));
    },
  };
  const tree = syntaxTree(state);
  for (const span of spans) {
    tree.iterate({
      from: span.from,
      to: span.to,
      enter: (ref) => {
        const ps = byNode.get(ref.name);
        if (!ps) return;
        // A node across two spans is entered twice; draw it once.
        if (spans.length > 1) {
          const k = `${ref.name}@${ref.from}`;
          if (seen.has(k)) return false;
          seen.add(k);
        }
        let descend = true;
        for (const p of ps) if (p.draw(ref, cx) === false) descend = false;
        return descend ? undefined : false;
      },
    });
  }
  return out;
}

/** buildInline draws the inline providers over spans: what the view plugin shows, exposed for tests. */
export function buildInline(state: EditorState, spans: readonly Span[]): DecorationSet {
  const ps = state.facet(previewProviders).filter((p) => p.kind === "inline");
  return Decoration.set(draw(state, spans, ps), true);
}

/** redraw asks the preview to draw again: link resolutions arrived, or the vault's files changed. */
export const redraw = StateEffect.define<null>();

const redrawn = (tr: Transaction) => tr.effects.some((e) => e.is(redraw));

const inlinePlugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = buildInline(view.state, view.visibleRanges);
    }
    update(u: ViewUpdate) {
      if (
        u.docChanged ||
        u.viewportChanged ||
        u.selectionSet ||
        syntaxTree(u.startState) !== syntaxTree(u.state) ||
        u.transactions.some(redrawn) ||
        u.startState.facet(previewEnv) !== u.state.facet(previewEnv)
      ) {
        this.decorations = buildInline(u.state, u.view.visibleRanges);
      }
    }
  },
  { decorations: (v) => v.decorations },
);

// topLevel returns the span of the document's top-level nodes that
// overlap from–to: a block's widget is redrawn whole.
function topLevel(state: EditorState, from: number, to: number): Span {
  const top = syntaxTree(state).topNode;
  let a = from;
  let b = to;
  for (let c = top.firstChild; c; c = c.nextSibling) {
    if (c.to < from) continue;
    if (c.from > to) break;
    a = Math.min(a, c.from);
    b = Math.max(b, c.to);
  }
  return { from: a, to: Math.min(b, state.doc.length) };
}

// blockSet draws the block providers over spans, sorted.
function blockSet(state: EditorState, spans: readonly Span[]): Range<Decoration>[] {
  const ps = state.facet(previewProviders).filter((p) => p.kind === "block");
  return draw(state, spans, ps);
}

/** blockField holds the block providers' decorations, redrawn only around changes and the selection. */
const blockField = StateField.define<DecorationSet>({
  create: (state) => Decoration.set(blockSet(state, [{ from: 0, to: state.doc.length }]), true),
  update(set, tr: Transaction) {
    const state = tr.state;
    if (!state.facet(previewProviders).some((p) => p.kind === "block")) return Decoration.none;
    const treeGrew = syntaxTree(tr.startState) !== syntaxTree(state) && !tr.docChanged;
    if (treeGrew || redrawn(tr) || tr.startState.facet(previewProviders) !== state.facet(previewProviders)) {
      return Decoration.set(blockSet(state, [{ from: 0, to: state.doc.length }]), true);
    }
    if (!tr.docChanged && !tr.selection) return set;
    const spans: Span[] = [];
    let mapped = set.map(tr.changes);
    tr.changes.iterChangedRanges((_fa, _ta, fromB, toB) => spans.push(topLevel(state, fromB, toB)));
    // The selection's old and new places: a block it enters shows its
    // source, and one it leaves draws again.
    for (const r of tr.startState.selection.ranges) {
      spans.push(topLevel(state, tr.changes.mapPos(r.from), tr.changes.mapPos(r.to)));
    }
    for (const r of state.selection.ranges) spans.push(topLevel(state, r.from, r.to));
    for (const s of spans) {
      mapped = mapped.update({ filter: (from, to) => to < s.from || from > s.to, filterFrom: s.from, filterTo: s.to });
    }
    const add = blockSet(state, spans).filter((d) => !overlapsKept(mapped, d));
    if (add.length === 0) return mapped;
    const b = new RangeSetBuilder<Decoration>();
    const all: Range<Decoration>[] = [];
    for (let it = mapped.iter(); it.value; it.next()) all.push(it.value.range(it.from, it.to));
    all.push(...add);
    all.sort((x, y) => x.from - y.from || x.value.startSide - y.value.startSide);
    for (const d of all) b.add(d.from, d.to, d.value);
    return b.finish();
  },
  provide: (f) => EditorView.decorations.from(f),
});

function overlapsKept(set: DecorationSet, d: Range<Decoration>): boolean {
  let hit = false;
  set.between(d.from, d.to, (from, to, value) => {
    if (from === d.from && to === d.to && value.eq(d.value)) hit = true;
  });
  return hit;
}

/** blockDecorations is the block providers' decorations in a state, for tests. */
export function blockDecorations(state: EditorState): DecorationSet {
  return state.field(blockField, false) ?? Decoration.none;
}

// clicks sends a click on an element a provider drew to that provider.
const clicks = EditorView.domEventHandlers({
  mousedown(e, view) {
    const el = e.target instanceof HTMLElement ? e.target.closest<HTMLElement>("[data-pv]") : null;
    if (!el || e.button > 1) return false;
    const p = view.state.facet(previewProviders).find((x) => x.id === el.dataset.pv);
    const action = p?.click?.(el, view, e);
    if (!action) return false;
    e.preventDefault();
    if (action.kind === "transaction") view.dispatch(action.spec);
    else view.state.facet(previewEnv).command(action.command);
    return true;
  },
});

/** livePreview is the framework over providers; source mode leaves it out. */
export function livePreview(providers: readonly Provider[]): Extension {
  return [providers.map((p) => previewProviders.of(p)), inlinePlugin, blockField, clicks];
}
