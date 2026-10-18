// The Tags pane's tree (docs/specs/ui.md, Right sidebar panes): nested
// tags with cumulative counts, preserving each segment's first spelling.
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
export function tagTree(counts: readonly TagCount[], sort: CountSort): TagNode[] {
  const roots: TagNode[] = [];
  const byPath = new Map<string, TagNode>();
  for (const { tag, count } of counts) {
    let parent: TagNode | undefined;
    for (const name of tag.split("/").filter(Boolean)) {
      const path = parent ? `${parent.tag}/${name}` : name;
      const key = path.toLowerCase();
      let node = byPath.get(key);
      if (!node) {
        node = { name, tag: path, count: 0, children: [] };
        byPath.set(key, node);
        (parent ? parent.children : roots).push(node);
      }
      node.count += count;
      parent = node;
    }
  }
  function sortNodes(nodes: TagNode[]): void {
    nodes.sort(
      (a, b) => (sort === "count" ? b.count - a.count : 0) || a.name.toLowerCase().localeCompare(b.name.toLowerCase()),
    );
    for (const node of nodes) sortNodes(node.children);
  }
  sortNodes(roots);
  return roots;
}
