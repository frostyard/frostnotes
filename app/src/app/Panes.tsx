// The sidebars' panes (docs/specs/ui.md#sidebars-and-panes): their tabs'
// names and icons, and their contents. Batch 2's cards draw the panes;
// until then Files lists the vault's files, opening them as links open,
// and the others are empty.
import { Archive, ArrowDownLeft, ArrowUpRight, FolderClosed, List, Search, Tags } from "lucide-react";
import type { ReactNode } from "react";

import { useFiles } from "../data/files";
import type { PaneId } from "../workspace/model";
import { activeTab, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { whereFor } from "./open";

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

/** Pane draws a pane's contents. */
export function Pane({ id }: { id: PaneId }) {
  return (
    <section aria-label={PANE_INFO[id].name} className="min-h-0 flex-1 overflow-y-auto">
      {id === "files" && <FileList />}
    </section>
  );
}

// FileList lists the vault's files by path, the open note's row selected.
function FileList() {
  const files = useFiles((s) => s.files);
  const update = useWorkspace((s) => s.update);
  const active = useWorkspace((s) => {
    const v = activeTab(s.ws).view;
    return v.type === "empty" ? undefined : v.path;
  });
  const paths = [...files.keys()].sort((a, b) => a.localeCompare(b));
  return (
    <ul className="py-1">
      {paths.map((p) => (
        <li key={p}>
          <button
            type="button"
            title={p}
            aria-current={p === active ? "page" : undefined}
            className={`text-sidebar-row w-full truncate px-4 py-0.5 text-left hover:bg-selection-inactive ${
              p === active ? "bg-selection-sidebar" : ""
            }`}
            onClick={(e) => openPath(p, whereFor(e))}
            onAuxClick={(e) => e.button === 1 && openPath(p, "tab")}
          >
            {p}
          </button>
        </li>
      ))}
    </ul>
  );

  function openPath(p: string, where: ReturnType<typeof whereFor>) {
    update((w) =>
      open(w, p.endsWith(".md") ? { type: "note", path: p, mode: "edit" } : { type: "file", path: p }, where),
    );
  }
}
