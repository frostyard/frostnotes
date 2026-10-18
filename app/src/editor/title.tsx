// The inline title in the editor (docs/specs/ui.md, Editor and reading
// view): T-0078's title, mounted with React in a block widget above the
// note's first line, in live preview and source mode alike, while the Show
// inline title preference is on. Its DOM is kept while the name changes.
import { Compartment, type Extension } from "@codemirror/state";
import { Decoration, EditorView, WidgetType } from "@codemirror/view";
import { createRoot, type Root } from "react-dom/client";

import { InlineTitle } from "../features/workspace/InlineTitle";

const roots = new WeakMap<HTMLElement, Root>();

class TitleWidget extends WidgetType {
  constructor(
    readonly title: string,
    readonly rename: (title: string) => void,
  ) {
    super();
  }
  override eq(o: TitleWidget): boolean {
    return o.title === this.title && o.rename === this.rename;
  }
  toDOM(view: EditorView): HTMLElement {
    const el = document.createElement("div");
    el.className = "cm-inline-title";
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
    if (root) queueMicrotask(() => root.unmount());
  }
  override ignoreEvent(): boolean {
    return true;
  }
  private paint(root: Root, view: EditorView) {
    root.render(<InlineTitle title={this.title} onRename={this.rename} onDone={() => view.focus()} />);
    queueMicrotask(() => view.requestMeasure());
  }
}

/** titleCompartment holds the inline title, so a view follows the preference and the note's name. */
export const titleCompartment = new Compartment();

/** inlineTitle is the title above the note, or nothing while the preference is off. */
export function inlineTitle(show: boolean, title: string, rename: (title: string) => void): Extension {
  if (!show) return [];
  const deco = Decoration.widget({ widget: new TitleWidget(title, rename), block: true, side: -1 });
  return EditorView.decorations.of(Decoration.set([deco.range(0)]));
}
