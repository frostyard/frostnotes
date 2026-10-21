// Which layout the window takes (docs/specs/ui.md#narrow-windows,
// ADR-0027): Obsidian's phone layout below 600 CSS pixels of width, the
// width of notes' frame in a browser. The query and the hook are
// lodge-ui's (@frostyard/lodge-ui/theme and ./react).
import { useNarrow } from "@frostyard/lodge-ui/react";
import { NARROW_QUERY } from "@frostyard/lodge-ui/theme";

/** NARROW is the media query of a narrow window, from lodge-ui. */
export const NARROW = NARROW_QUERY;

export { useNarrow };
