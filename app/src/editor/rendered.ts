// Live preview's rendered blocks (callouts, tables, embeds;
// docs/design/app.md#live-preview): markdown that notesd renders as the
// note would render it (note.render with content), sanitized as the
// reading view sanitizes it, its files' URLs and math filled in by the
// reading view's finish. A note's editor has one renderer: it keeps what
// notesd answered by markdown, and fills its elements again when the
// vault changes, keeping what they show until the new answer arrives.

/** RENDER_CACHE is how many renderings a renderer keeps. */
export const RENDER_CACHE = 200;

/** BlockRenderer renders markdown into live preview's elements. */
export class BlockRenderer {
  private readonly cache = new Map<string, string>();
  private readonly pending = new Map<string, Promise<string>>();
  private readonly shown = new Map<HTMLElement, string>();

  /**
   * ask renders markdown to sanitized HTML; place shows it in an element,
   * finished (the reading view's fill: files, math); filled tells the view
   * an element's size changed.
   */
  constructor(
    private readonly ask: (markdown: string) => Promise<string>,
    private readonly place: (el: HTMLElement, html: string) => void,
    private readonly filled: () => void,
  ) {}

  /** fill shows markdown in el: at once from the cache, else when notesd answers. */
  fill = (el: HTMLElement, markdown: string): void => {
    this.shown.set(el, markdown);
    const html = this.cache.get(markdown);
    if (html !== undefined) {
      this.put(el, html);
      return;
    }
    this.load(markdown).then(
      (h) => {
        if (this.shown.get(el) === markdown) this.put(el, h);
      },
      (err: unknown) => {
        if (this.shown.get(el) !== markdown) return;
        el.textContent = `Cannot render this: ${err instanceof Error ? err.message : String(err)}`;
        this.filled();
      },
    );
  };

  /** refresh forgets every answer and fills what is on screen again: the vault changed. */
  refresh(): void {
    this.cache.clear();
    for (const [el, markdown] of [...this.shown]) {
      if (el.isConnected) this.fill(el, markdown);
      else this.shown.delete(el);
    }
  }

  private load(markdown: string): Promise<string> {
    let p = this.pending.get(markdown);
    if (p) return p;
    p = this.ask(markdown).then((html) => {
      this.cache.set(markdown, html);
      while (this.cache.size > RENDER_CACHE) {
        const oldest = this.cache.keys().next().value;
        if (oldest === undefined) break;
        this.cache.delete(oldest);
      }
      return html;
    });
    const settle = () => this.pending.delete(markdown);
    p.then(settle, settle);
    this.pending.set(markdown, p);
    return p;
  }

  private put(el: HTMLElement, html: string): void {
    this.place(el, html);
    this.filled();
  }
}
