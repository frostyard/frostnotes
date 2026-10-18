// The sidebars' panes (docs/specs/ui.md#sidebars-and-panes): their tabs'
// names and icons, and their containers (app/panes/), which draw batch
// 2's panes over the window's vault and active note.
import { Archive, ArrowDownLeft, ArrowUpRight, FolderClosed, List, Search, Tags } from "lucide-react";
import type { ReactNode } from "react";

import type { PaneId } from "../workspace/model";
import { PropertiesContainer, TagsContainer } from "./panes/counts";
import { FilesContainer } from "./panes/files";
import { BacklinksContainer, OutgoingContainer } from "./panes/links";
import { OutlineContainer } from "./panes/outline";
import { SearchContainer } from "./panes/search";

/** PANE_INFO names each pane and gives its icon. */
export const PANE_INFO: Record<PaneId, { name: string; icon: ReactNode }> = {
  files: { name: "Files", icon: <FolderClosed size={16} /> },
  search: { name: "Search", icon: <Search size={16} /> },
  backlinks: { name: "Backlinks", icon: <ArrowDownLeft size={16} /> },
  outgoing: { name: "Outgoing links", icon: <ArrowUpRight size={16} /> },
  tags: { name: "Tags", icon: <Tags size={16} /> },
  properties: { name: "All properties", icon: <Archive size={16} /> },
  outline: { name: "Outline", icon: <List size={16} /> },
};

const CONTAINERS: Record<PaneId, () => ReactNode> = {
  files: () => <FilesContainer />,
  search: () => <SearchContainer />,
  backlinks: () => <BacklinksContainer />,
  outgoing: () => <OutgoingContainer />,
  tags: () => <TagsContainer />,
  properties: () => <PropertiesContainer />,
  outline: () => <OutlineContainer />,
};

/** Pane draws a pane's contents. */
export function Pane({ id }: { id: PaneId }) {
  return (
    <section aria-label={PANE_INFO[id].name} className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      {CONTAINERS[id]()}
    </section>
  );
}
