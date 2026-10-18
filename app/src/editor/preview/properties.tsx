// The properties block in live preview (docs/design/app.md#properties;
// docs/specs/ui.md, Editor and reading view): the frontmatter replaced by
// T-0077's block, mounted with React in a block widget whose DOM is kept
// while the frontmatter changes, so a field keeps its focus. Each change
// is an edit of the YAML text (editor/properties.ts) that keeps every
// other byte, dispatched as one change. The Properties in document
// preference hides the frontmatter or leaves its YAML instead. The block
// takes its own events: the editor never sees them.
import { StateEffect, StateField } from "@codemirror/state";
import { Decoration, type EditorView, WidgetType } from "@codemirror/view";
import { createRoot, type Root } from "react-dom/client";

import { notify } from "../../data/overlays";
import { PropertiesBlock, type PropertyRow } from "../../features/properties/PropertiesBlock";
import { propertyType, readFrontmatter, renameProperty, setProperties } from "../properties";
import { type Provider, redraw } from "./framework";

/** foldProperties folds (true) or unfolds the block. */
export const foldProperties = StateEffect.define<boolean>();
/** addProperty opens the block's new row (Add file property). */
export const addProperty = StateEffect.define<null>();

interface BlockState {
  folded: boolean;
  addRequest: number;
}

/** propertiesState is the block's fold and how often a new row was asked for, per view. */
export const propertiesState = StateField.define<BlockState>({
  create: () => ({ folded: false, addRequest: 0 }),
  update(value, tr) {
    let v = value;
    for (const e of tr.effects) {
      if (e.is(foldProperties)) v = { ...v, folded: e.value };
      else if (e.is(addProperty)) v = { ...v, addRequest: v.addRequest + 1 };
    }
    return v;
  },
});

/** textChange is the one change that makes prev next: the span between their common start and end. */
export function textChange(prev: string, next: string): { from: number; to: number; insert: string } {
  let a = 0;
  while (a < prev.length && a < next.length && prev[a] === next[a]) a++;
  let b = 0;
  while (b < prev.length - a && b < next.length - a && prev[prev.length - 1 - b] === next[next.length - 1 - b]) b++;
  return { from: a, to: prev.length - b, insert: next.slice(a, next.length - b) };
}

/** replaceText makes the view's document next with one change. */
export function replaceText(view: EditorView, next: string, userEvent: string): void {
  const prev = view.state.doc.toString();
  if (prev !== next) view.dispatch({ changes: textChange(prev, next), userEvent });
}

function apply(view: EditorView, next: string | Error) {
  if (next instanceof Error) notify(`Cannot change the properties: ${next.message}`);
  else replaceText(view, next, "input.properties");
}

const roots = new WeakMap<HTMLElement, Root>();

class PropertiesWidget extends WidgetType {
  constructor(
    readonly front: string,
    readonly folded: boolean,
    readonly addRequest: number,
    readonly types: Readonly<Record<string, string>>,
  ) {
    super();
  }
  override eq(o: PropertiesWidget): boolean {
    return (
      o.front === this.front && o.folded === this.folded && o.addRequest === this.addRequest && o.types === this.types
    );
  }
  toDOM(view: EditorView): HTMLElement {
    const el = document.createElement("div");
    el.className = "cm-pv-properties";
    const root = createRoot(el);
    roots.set(el, root);
    this.paint(root, view);
    return el;
  }
  override updateDOM(dom: HTMLElement, view: EditorView): boolean {
    const root = roots.get(dom);
    if (!root) return false;
    this.paint(root, view);
    return true;
  }
  override destroy(dom: HTMLElement): void {
    const root = roots.get(dom);
    roots.delete(dom);
    // Not inside the editor's update, which React may be rendering from.
    if (root) queueMicrotask(() => root.unmount());
  }
  override ignoreEvent(): boolean {
    return true;
  }
  private paint(root: Root, view: EditorView) {
    const fm = readFrontmatter(this.front);
    const rows: PropertyRow[] =
      fm.kind === "properties"
        ? fm.properties.map((p) => ({
            key: p.key,
            value: p.value,
            type: propertyType(p.key, p.value, this.types[p.key.toLowerCase()] ?? ""),
          }))
        : [];
    const text = () => view.state.doc.toString();
    root.render(
      <PropertiesBlock
        rows={rows}
        error={fm.kind === "error" ? fm.message : null}
        folded={this.folded}
        addRequest={this.addRequest}
        onFold={(folded) => view.dispatch({ effects: [foldProperties.of(folded), redraw.of(null)] })}
        onChange={(key, value) => apply(view, setProperties(text(), { [key]: value }))}
        onRename={(from, to) => apply(view, renameProperty(text(), from, to))}
        onRemove={(key) => apply(view, setProperties(text(), { [key]: null }))}
      />,
    );
    // The block's height is React's to settle.
    queueMicrotask(() => view.requestMeasure());
  }
}

/** propertiesBlock draws the frontmatter as the properties block, hides it, or leaves it, by the preference. */
export const propertiesBlock: Provider = {
  id: "properties",
  kind: "block",
  nodes: ["Frontmatter"],
  draw(node, cx) {
    const mode = cx.env.propertiesMode();
    if (mode === "source") return false;
    const doc = cx.state.doc;
    const from = doc.lineAt(node.from).from;
    const to = doc.lineAt(node.to).to;
    if (mode === "hidden") {
      cx.add(Decoration.replace({ block: true }).range(from, to));
      return false;
    }
    const s = cx.state.field(propertiesState, false) ?? { folded: false, addRequest: 0 };
    const widget = new PropertiesWidget(doc.sliceString(from, to), s.folded, s.addRequest, cx.env.propertyTypes());
    cx.add(Decoration.replace({ widget, block: true }).range(from, to));
    return false;
  },
};

/** PROPERTIES are the providers of the properties block. */
export const PROPERTIES: readonly Provider[] = [propertiesBlock];
