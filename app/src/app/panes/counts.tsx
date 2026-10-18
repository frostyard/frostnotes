// The Tags and All properties panes' containers (docs/specs/ui.md, Right
// sidebar panes): the vault's counts from note.tags and note.properties,
// read again as the vault's files change; a tag searches "tag:#name", a
// key searches "[key]", which the Search pane lists through note.list.
import { useEffect, useMemo, useState } from "react";

import { useFiles } from "../../data/files";
import { openMenu } from "../../data/overlays";
import { setSearch, usePanes } from "../../data/panes";
import { useClient } from "../../data/session";
import { useNotes } from "../../data/stores";
import type { CountSort } from "../../features/panes/common";
import { PropertiesPane } from "../../features/properties/PropertiesPane";
import { TagsPane } from "../../features/tags/TagsPane";
import { tagTree } from "../../lib/tagTree";
import type { PropertyCount, TagCount } from "../../rpc/gen/api";
import { selectPane } from "../../workspace/ops";
import { useWorkspace } from "../../workspace/store";

// useCounts reads a vault-wide list, again whenever the vault's files change.
function useCounts<T>(read: (vaultId: number) => Promise<T[]>): T[] | null {
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  const [list, setList] = useState<{ vaultId: number; items: T[] } | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger; read is the caller's fixed query
  useEffect(() => {
    if (vaultId === null) return;
    let stale = false;
    read(vaultId)
      .then((items) => !stale && setList({ vaultId, items }))
      .catch(() => !stale && setList({ vaultId, items: [] }));
    return () => {
      stale = true;
    };
  }, [vaultId, files]);
  return list !== null && list.vaultId === vaultId ? list.items : null;
}

// sortMenu opens the name-or-count menu at (x, y).
function sortMenu(current: CountSort, x: number, y: number, set: (s: CountSort) => void): void {
  openMenu({
    label: "Sort order",
    x,
    y,
    items: [
      { kind: "item", id: "name", label: "Name (A to Z)", checked: current === "name" },
      { kind: "item", id: "count", label: "Count (most first)", checked: current === "count" },
    ],
    onChoose: (id) => set(id as CountSort),
  });
}

/** searchFor shows the Search pane with a query. */
export function searchFor(query: string): void {
  setSearch(query, true);
  useWorkspace.getState().update((w) => selectPane(w, "left", "search"));
}

/** TagsContainer draws the Tags pane over the window's vault. */
export function TagsContainer() {
  const client = useClient();
  const sort = usePanes((s) => s.tagSort);
  const counts = useCounts<TagCount>((vaultId) => client.note.tags({ vaultId }));
  const tree = useMemo(() => (counts === null ? null : tagTree(counts, sort)), [counts, sort]);
  return (
    <TagsPane
      tags={tree}
      onSortMenu={(x, y) => sortMenu(sort, x, y, (s) => usePanes.setState({ tagSort: s }))}
      onChoose={(tag) => searchFor(`tag:#${tag}`)}
    />
  );
}

/** PropertiesContainer draws the All properties pane over the window's vault. */
export function PropertiesContainer() {
  const client = useClient();
  const sort = usePanes((s) => s.propertySort);
  const counts = useCounts<PropertyCount>((vaultId) => client.note.properties({ vaultId }));
  return (
    <PropertiesPane
      properties={counts}
      sort={sort}
      onSortMenu={(x, y) => sortMenu(sort, x, y, (s) => usePanes.setState({ propertySort: s }))}
      onChoose={(key) => searchFor(`[${key}]`)}
    />
  );
}
