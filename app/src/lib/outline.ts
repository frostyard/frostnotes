// The Outline pane's tree (docs/specs/ui.md, Right sidebar panes): the
// active note's headings, nested as Obsidian's outline nests them, its rows
// with some headings folded, and the heading the cursor is under. The
// editor reads the headings from its syntax tree as the user types. Task
// T-0041 implements it; until then the outline is empty.

/** Heading is a heading of the note. */
export interface Heading {
  /** 1 to 6. */
  level: number;
  /** The heading's text as the outline shows it. */
  text: string;
  /** Where the heading's line starts in the document; headings come in document order. */
  from: number;
}

/** OutlineNode is a heading and the headings nested under it. */
export interface OutlineNode {
  heading: Heading;
  children: OutlineNode[];
}

/** OutlineRow is one row of the Outline pane. */
export interface OutlineRow {
  heading: Heading;
  /** 0 for a heading nested under none. */
  depth: number;
  hasChildren: boolean;
  /** Its children show: it has some and is not folded. */
  expanded: boolean;
}

/** buildOutline nests headings: each under the nearest heading before it of a lower level. */
export function buildOutline(_headings: readonly Heading[]): OutlineNode[] {
  return [];
}

/** outlineRows flattens an outline into rows, leaving out what folded headings (by from) hold. */
export function outlineRows(_nodes: readonly OutlineNode[], _folded: ReadonlySet<number>): OutlineRow[] {
  return [];
}

/** headingAt returns the index of the heading whose section holds pos, or -1 before the first heading. */
export function headingAt(_headings: readonly Heading[], _pos: number): number {
  return -1;
}
