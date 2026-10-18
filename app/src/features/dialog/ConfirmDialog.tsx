// A confirmation (docs/specs/ui.md, Files pane: Delete asks first). Task
// T-0062 replaces this stub.

export interface ConfirmDialogProps {
  title: string;
  text: string;
  /** The confirming button's label ("Delete"). */
  confirmLabel: string;
  /** The cancelling button's label; "Cancel" by default. */
  cancelLabel?: string;
  /** The confirming button destroys something: drawn in the red flag's color. */
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/** ConfirmDialog draws a confirmation in a portal on the document's body. */
export function ConfirmDialog(_props: ConfirmDialogProps) {
  return null;
}
