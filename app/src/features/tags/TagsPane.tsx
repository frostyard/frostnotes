// The Tags pane (docs/specs/ui.md, Right sidebar panes): expandable tag
// counts with callbacks for searching a tag and choosing the sort order.

import { IconButton } from "@frostyard/lodge-ui/react";
import { ArrowUpNarrowWide, ChevronRight } from "lucide-react";
import { useState } from "react";
import type { TagNode } from "../../lib/tagTree";
import { paneEmpty, paneRow, paneToolbar } from "../panes/common";

export interface TagsPaneProps {
  /** The tree; null while it loads. */
  tags: readonly TagNode[] | null;
  /** Open the sort order's menu at (x, y): the button's left and bottom. */
  onSortMenu: (x: number, y: number) => void;
  /** Search for the tag (its whole path, without "#"). */
  onChoose: (tag: string) => void;
}

/** TagsPane draws the Tags pane. */
export function TagsPane({ tags, onSortMenu, onChoose }: TagsPaneProps) {
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  function toggle(tag: string): void {
    const key = tag.toLowerCase();
    setExpanded((previous) => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }
  return (
    <>
      <div className={paneToolbar}>
        <IconButton
          label="Change sort order"
          size={24}
          icon={ArrowUpNarrowWide}
          onClick={(event) => {
            const { left, bottom } = event.currentTarget.getBoundingClientRect();
            onSortMenu(left, bottom);
          }}
        />
      </div>
      {tags !== null &&
        (tags.length === 0 ? (
          <p className={paneEmpty}>No tags found.</p>
        ) : (
          <div role="tree" aria-label="Tags">
            {tags.map((node) => (
              <TagRows key={node.tag.toLowerCase()} {...{ node, expanded, onChoose, toggle }} depth={0} />
            ))}
          </div>
        ))}
    </>
  );
}

interface TagRowsProps {
  node: TagNode;
  depth: number;
  expanded: ReadonlySet<string>;
  toggle: (tag: string) => void;
  onChoose: (tag: string) => void;
}

function TagRows({ node, depth, expanded, toggle, onChoose }: TagRowsProps) {
  const hasChildren = node.children.length > 0;
  const open = expanded.has(node.tag.toLowerCase());
  return (
    <>
      <div
        role="treeitem"
        tabIndex={-1}
        aria-level={depth + 1}
        aria-expanded={hasChildren ? open : undefined}
        className={paneRow}
        style={{ paddingLeft: 4 + 16 * depth }}
      >
        <div className="flex w-4 shrink-0 items-center justify-center">
          {hasChildren && (
            <button
              type="button"
              tabIndex={-1}
              aria-label={`${open ? "Collapse" : "Expand"} #${node.tag}`}
              onClick={() => toggle(node.tag)}
              className="flex size-4 items-center justify-center text-secondary"
            >
              <ChevronRight size={14} className={open ? "rotate-90" : undefined} />
            </button>
          )}
        </div>
        <button
          type="button"
          title={`#${node.tag}`}
          onClick={() => onChoose(node.tag)}
          className="flex h-full min-w-0 flex-1 items-center justify-between gap-2 pr-2 text-left"
        >
          <span className="truncate">{node.name}</span>
          <span className="text-status shrink-0 text-secondary">{node.count}</span>
        </button>
      </div>
      {open &&
        node.children.map((child) => (
          <TagRows key={child.tag.toLowerCase()} node={child} depth={depth + 1} {...{ expanded, toggle, onChoose }} />
        ))}
    </>
  );
}
