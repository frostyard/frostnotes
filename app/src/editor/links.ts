// Links in the editor (docs/design/app.md#extensions): note.resolve
// resolves the targets live preview draws from the note's folder, as
// notesd's index does, in one batched call per moment, cached until a file
// is added, moved or removed. An unresolved link shows dimmed.
import type { Resolution } from "../rpc/gen/api";

/** LinkResolver answers targets from its cache and asks notesd for the rest, together. */
export class LinkResolver {
  private readonly cache = new Map<string, string | null>();
  private readonly wanted = new Set<string>();
  private scheduled = false;
  private generation = 0;

  /**
   * ask resolves targets from the note; changed tells the views to draw
   * again once answers arrive.
   */
  constructor(
    private readonly ask: (targets: string[]) => Promise<Resolution[]>,
    private readonly changed: () => void,
  ) {}

  /** resolve gives a target's path, null when unresolved, undefined while asked. */
  resolve = (target: string): string | null | undefined => {
    if (target === "") return null;
    if (this.cache.has(target)) return this.cache.get(target);
    this.wanted.add(target);
    if (!this.scheduled) {
      this.scheduled = true;
      queueMicrotask(() => void this.flush());
    }
    return undefined;
  };

  /** reset forgets every answer: the vault's files changed, or the note moved. */
  reset(): void {
    this.cache.clear();
    this.generation++;
    this.changed();
  }

  private async flush(): Promise<void> {
    this.scheduled = false;
    const targets = [...this.wanted].slice(0, 1000);
    for (const t of targets) this.wanted.delete(t);
    if (targets.length === 0) return;
    const generation = this.generation;
    try {
      const answers = await this.ask(targets);
      if (generation !== this.generation) return;
      for (const a of answers) this.cache.set(a.target, a.path ?? null);
      this.changed();
    } catch {
      // Unanswered targets draw as resolved; the next change asks again.
    }
    if (this.wanted.size > 0 && !this.scheduled) {
      this.scheduled = true;
      queueMicrotask(() => void this.flush());
    }
  }
}
