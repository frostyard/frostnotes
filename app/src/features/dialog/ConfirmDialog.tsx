// A confirmation (docs/specs/ui.md, Files pane: Delete asks first), with
// themed actions, isolated keyboard events, and focus returned on close.
import { Sheet } from "@frostyard/lodge-ui/react";
import { useRef } from "react";

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

/** ConfirmDialog is lodge-ui's Sheet as an alertdialog, portalled onto the document's body. */
export function ConfirmDialog(props: ConfirmDialogProps) {
  const { title, text, confirmLabel, cancelLabel = "Cancel", danger, onConfirm, onCancel } = props;
  const confirmButton = useRef<HTMLButtonElement>(null);

  return (
    <Sheet
      title={title}
      description={text}
      role="alertdialog"
      width={400}
      showClose={false}
      initialFocus={confirmButton}
      portal
      onClose={onCancel}
    >
      <div className="flex justify-end gap-2">
        <button
          type="button"
          className="h-7 rounded-md border border-separator px-3 hover:bg-selection-inactive"
          onClick={onCancel}
        >
          {cancelLabel}
        </button>
        <button
          ref={confirmButton}
          type="button"
          className={`h-7 rounded-md px-3 font-medium text-accent-contrast ${danger ? "bg-flag-1" : "bg-accent"}`}
          onClick={onConfirm}
        >
          {confirmLabel}
        </button>
      </div>
    </Sheet>
  );
}
