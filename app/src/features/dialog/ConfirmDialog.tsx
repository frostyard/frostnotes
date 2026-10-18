// A confirmation (docs/specs/ui.md, Files pane: Delete asks first), with
// themed actions, isolated keyboard events, and focus returned on close.
import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";

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
export function ConfirmDialog(props: ConfirmDialogProps) {
  const { title, text, confirmLabel, cancelLabel = "Cancel", danger, onConfirm, onCancel } = props;
  const titleId = useId();
  const textId = useId();
  const confirmButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const before = document.activeElement;
    confirmButton.current?.focus();
    return () => {
      if (before instanceof HTMLElement && before.isConnected) before.focus();
    };
  }, []);

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-backdrop"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) onCancel();
      }}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={textId}
        className="w-[400px] max-w-[90%] rounded-xl border border-separator bg-window p-5 shadow-popover"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            onCancel();
          }
        }}
      >
        <h2 id={titleId} className="font-semibold text-empty">
          {title}
        </h2>
        <p id={textId} className="mt-2 text-secondary text-sidebar-row">
          {text}
        </p>
        <div className="mt-5 flex justify-end gap-2">
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
      </div>
    </div>,
    document.body,
  );
}
