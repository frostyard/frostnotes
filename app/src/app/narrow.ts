// Which layout the window takes (docs/specs/ui.md#narrow-windows,
// ADR-0027): Obsidian's phone layout below 600 CSS pixels of width, the
// width of notes' frame in a browser.
import { useSyncExternalStore } from "react";

/** NARROW is the media query of a narrow window. */
export const NARROW = "(max-width: 599px)";

function query(): MediaQueryList | null {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia(NARROW) : null;
}

/** useNarrow reports whether the window is narrow, following its width. */
export function useNarrow(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const q = query();
      q?.addEventListener("change", onChange);
      return () => q?.removeEventListener("change", onChange);
    },
    () => query()?.matches ?? false,
  );
}
