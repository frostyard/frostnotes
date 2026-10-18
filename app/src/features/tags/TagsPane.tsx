// The Tags pane (docs/specs/ui.md, Right sidebar panes). Task T-0055
// replaces this stub.
import type { TagNode } from "../../lib/tagTree";

export interface TagsPaneProps {
  /** The tree; null while it loads. */
  tags: readonly TagNode[] | null;
  /** Open the sort order's menu at (x, y): the button's left and bottom. */
  onSortMenu: (x: number, y: number) => void;
  /** Search for the tag (its whole path, without "#"). */
  onChoose: (tag: string) => void;
}

/** TagsPane draws the Tags pane. */
export function TagsPane(_props: TagsPaneProps) {
  return null;
}
