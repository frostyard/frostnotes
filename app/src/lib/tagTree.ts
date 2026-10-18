// The Tags pane's tree (docs/specs/ui.md, Right sidebar panes). Task
// T-0055 replaces this stub.
import type { CountSort } from "../features/panes/common";
import type { TagCount } from "../rpc/gen/api";

/** TagNode is a tag and the tags nested under it. */
export interface TagNode {
  /** The tag's last segment, as first spelled. */
  name: string;
  /** The whole tag, its segments as first spelled, without "#" ("garden/fruit"). */
  tag: string;
  /** The notes with the tag, plus its children's counts. */
  count: number;
  children: TagNode[];
}

/** tagTree nests counts into a tree, sorted by sort at every level. */
export function tagTree(_counts: readonly TagCount[], _sort: CountSort): TagNode[] {
  return [];
}
