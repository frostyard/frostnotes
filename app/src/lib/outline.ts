// The Outline pane's tree (docs/specs/ui.md, Right sidebar panes): the
// active note's headings, nested as Obsidian's outline nests them, its rows
// with some headings folded, and the heading the cursor is under. The
// editor reads the headings from its syntax tree as the user types.

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
export function buildOutline(headings: readonly Heading[]): OutlineNode[] {
  const roots: OutlineNode[] = [];
  const stack: OutlineNode[] = [];
  for (const heading of headings) {
    let parent = stack.at(-1);
    while (parent && parent.heading.level >= heading.level) {
      stack.pop();
      parent = stack.at(-1);
    }
    const node: OutlineNode = { heading, children: [] };
    (parent ? parent.children : roots).push(node);
    stack.push(node);
  }
  return roots;
}

/** outlineRows flattens an outline into rows, leaving out what folded headings (by from) hold. */
export function outlineRows(nodes: readonly OutlineNode[], folded: ReadonlySet<number>): OutlineRow[] {
  const rows: OutlineRow[] = [];
  const visit = (children: readonly OutlineNode[], depth: number): void => {
    for (const node of children) {
      const hasChildren = node.children.length > 0;
      const expanded = hasChildren && !folded.has(node.heading.from);
      rows.push({ heading: node.heading, depth, hasChildren, expanded });
      if (expanded) visit(node.children, depth + 1);
    }
  };
  visit(nodes, 0);
  return rows;
}

/** headingAt returns the index of the heading whose section holds pos, or -1 before the first heading. */
export function headingAt(headings: readonly Heading[], pos: number): number {
  let low = 0;
  let high = headings.length;
  while (low < high) {
    const mid = Math.floor((low + high) / 2);
    const heading = headings[mid];
    if (heading && heading.from <= pos) {
      low = mid + 1;
    } else {
      high = mid;
    }
  }
  return low - 1;
}
