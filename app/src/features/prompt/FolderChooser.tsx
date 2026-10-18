// The folder chooser (docs/specs/ui.md, Files pane: Move file to…). Task
// T-0063 replaces this stub.

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
export function FolderChooser(_props: FolderChooserProps) {
  return null;
}
