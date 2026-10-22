// The folder chooser (docs/specs/ui.md, Files pane: Move file to…): a
// searchable prompt over the vault's folders, including its root.

import { highlight, Prompt } from "@frostyard/lodge-ui/react";
import { useMemo, useState } from "react";
import { type FolderResult, folderResults, folderText } from "../../lib/folders";

export interface FolderChooserProps {
  /** Every folder of the vault, the root not among them. */
  folders: readonly string[];
  /** What the field says before anything is typed ('Move "Garden plan" to…'). */
  placeholder: string;
  /** The folder chosen: "" for the vault's root. */
  onChoose: (folder: string) => void;
  onClose: () => void;
}

/** FolderChooser draws the folder chooser. */
export function FolderChooser({ folders, placeholder, onChoose, onClose }: FolderChooserProps) {
  const [query, setQuery] = useState("");
  const items = useMemo(() => folderResults(folders, query), [folders, query]);

  function onEnter(item: FolderResult | undefined) {
    if (!item) return;
    onClose();
    onChoose(item.path);
  }

  return (
    <Prompt
      label="Move to folder"
      placeholder={placeholder}
      query={query}
      onQuery={setQuery}
      items={items}
      itemKey={(item) => `folder:${item.path}`}
      renderItem={({ path, match }) => (
        <span className="truncate text-sidebar-row">{highlight(folderText(path), match?.matches ?? [])}</span>
      )}
      onEnter={onEnter}
      onClose={onClose}
      instructions={[
        { keys: "↑↓", text: "to navigate" },
        { keys: "↵", text: "to move" },
        { keys: "esc", text: "to dismiss" },
      ]}
      emptyText="No folders found."
    />
  );
}
