// The settings dialog (docs/specs/ui.md, Settings): section navigation,
// editable controls and read-only values in a focused modal frame. In a
// narrow window (docs/specs/ui.md#narrow-windows) it is full screen: the
// list of sections, then a section over it with a back button.
import { ChevronLeft, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

/** SettingControl is a setting's control and value. */
export type SettingControl =
  | { kind: "toggle"; value: boolean }
  | { kind: "select"; value: string; options: readonly { value: string; label: string }[] }
  | { kind: "text"; value: string; placeholder?: string }
  | { kind: "number"; value: number; min: number; max: number }
  /** Read-only key caps (the Hotkeys section). */
  | { kind: "keys"; keys: readonly string[] }
  /** Read-only text. */
  | { kind: "info"; text: string }
  /** An action: a button with its label, red when it cannot be undone; a click is onChange(id, true). */
  | { kind: "button"; label: string; danger?: boolean };

/** SettingRow is one setting. */
export interface SettingRow {
  id: string;
  name: string;
  description?: string;
  control: SettingControl;
  disabled?: boolean;
}

/** SettingsSection is one section of the dialog. */
export interface SettingsSection {
  id: string;
  name: string;
  rows: readonly SettingRow[];
}

export interface SettingsDialogProps {
  sections: readonly SettingsSection[];
  /** The shown section's id. */
  section: string;
  onSection: (id: string) => void;
  /** A setting changed: a toggle's boolean, a select's or text's string, a number's number. */
  onChange: (id: string, value: boolean | string | number) => void;
  onClose: () => void;
  /** A narrow window's: the list of sections first, each section over it with Back. */
  narrow?: boolean;
}

/** SettingsDialog draws the settings dialog in a portal on the document's body. */
export function SettingsDialog({
  sections,
  section,
  onSection,
  onChange,
  onClose,
  narrow = false,
}: SettingsDialogProps) {
  const dialog = useRef<HTMLDivElement>(null);
  const current = sections.find((entry) => entry.id === section) ?? sections[0];
  // A narrow window shows the list until a section is chosen.
  const [listing, setListing] = useState(narrow);

  useEffect(() => {
    const before = document.activeElement;
    dialog.current?.focus();
    return () => {
      if (before instanceof HTMLElement && before.isConnected) before.focus();
    };
  }, []);

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-backdrop"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-label="Settings"
        tabIndex={-1}
        className="relative flex h-[640px] max-h-[85vh] w-[900px] max-w-[90vw] overflow-hidden rounded-xl border border-separator bg-window shadow-popover max-[599px]:h-[100dvh] max-[599px]:max-h-[100dvh] max-[599px]:w-full max-[599px]:max-w-full max-[599px]:flex-col max-[599px]:rounded-none max-[599px]:border-0"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            onClose();
          }
        }}
      >
        {narrow && (
          <div className="flex h-11 shrink-0 items-center border-b border-separator px-1">
            {!listing && (
              <button
                type="button"
                aria-label="Back"
                className="flex size-11 items-center justify-center text-secondary"
                onClick={() => setListing(true)}
              >
                <ChevronLeft size={20} aria-hidden="true" />
              </button>
            )}
            <h2 className={`min-w-0 flex-1 truncate font-semibold ${listing ? "px-3" : ""}`}>
              {listing ? "Settings" : current?.name}
            </h2>
          </div>
        )}
        {(!narrow || listing) && (
          <nav
            aria-label="Settings sections"
            className={`shrink-0 overflow-y-auto bg-sidebar p-2 ${narrow ? "min-h-0 flex-1" : "w-[200px]"}`}
          >
            {sections.map((entry) => (
              <button
                key={entry.id}
                type="button"
                aria-current={entry.id === current?.id && !narrow ? "page" : undefined}
                className={`block w-full rounded-md px-2 text-left text-sidebar-row ${narrow ? "h-11" : "h-7 pointer-coarse:h-11"} ${entry.id === current?.id && !narrow ? "bg-selection-sidebar" : "hover:bg-selection-inactive"}`}
                onClick={() => {
                  onSection(entry.id);
                  setListing(false);
                }}
              >
                {entry.name}
              </button>
            ))}
          </nav>
        )}
        {current && (!narrow || !listing) && (
          <Section key={current.id} section={current} onChange={onChange} titled={!narrow} />
        )}
        <button
          type="button"
          aria-label="Close"
          className="absolute top-2 right-2 flex size-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive max-[599px]:top-0 max-[599px]:right-1 max-[599px]:size-11 pointer-coarse:size-11"
          onClick={onClose}
        >
          <X size={16} aria-hidden="true" />
        </button>
      </div>
    </div>,
    document.body,
  );
}

function Section({
  section,
  onChange,
  titled,
}: {
  section: SettingsSection;
  onChange: SettingsDialogProps["onChange"];
  /** Whether the section draws its own title: a narrow window's header names it instead. */
  titled: boolean;
}) {
  return (
    <section aria-label={section.name} className="min-h-0 min-w-0 flex-1 overflow-y-auto p-6 max-[599px]:p-4">
      {titled && <h2 className="pr-6 text-empty font-semibold">{section.name}</h2>}
      {section.rows.map((row) => (
        <div key={row.id} data-setting={row.id} className="flex items-center gap-6 border-b border-separator py-3">
          <div className="min-w-0 flex-1">
            <div className="text-sidebar-row">{row.name}</div>
            {row.description !== undefined && <p className="text-xs leading-4 text-secondary">{row.description}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Control row={row} onChange={onChange} />
          </div>
        </div>
      ))}
    </section>
  );
}

function Control({ row, onChange }: { row: SettingRow; onChange: SettingsDialogProps["onChange"] }) {
  const { id, name, control, disabled } = row;
  switch (control.kind) {
    case "toggle":
      return (
        <button
          type="button"
          role="switch"
          aria-checked={control.value}
          aria-label={name}
          disabled={disabled}
          className={`flex h-5 w-9 items-center rounded-full p-0.5 disabled:opacity-50 ${control.value ? "justify-end bg-accent" : "justify-start bg-badge"}`}
          onClick={() => onChange(id, !control.value)}
        >
          <span className="size-4 rounded-full bg-window" />
        </button>
      );
    case "select":
      return (
        <select
          aria-label={name}
          value={control.value}
          disabled={disabled}
          className="h-7 w-[200px] rounded-md border border-separator bg-window px-2 focus:border-accent disabled:opacity-50"
          onChange={(event) => onChange(id, event.target.value)}
        >
          {control.options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      );
    case "text":
    case "number":
      return <DraftField key={control.kind} row={row} control={control} onChange={onChange} />;
    case "keys":
      return control.keys.map((key) => (
        <kbd key={key} className="rounded bg-badge px-1.5 py-0.5 font-sans text-keycap text-badge-text">
          {key}
        </kbd>
      ));
    case "info":
      return <span className="select-text text-secondary">{control.text}</span>;
    case "button":
      return (
        <button
          type="button"
          aria-label={`${control.label} ${name}`}
          disabled={disabled}
          className={`h-7 rounded-md border border-separator px-3 disabled:opacity-50 ${control.danger ? "text-flag-1" : "text-primary"}`}
          onClick={() => onChange(id, true)}
        >
          {control.label}
        </button>
      );
  }
}

function DraftField({
  row,
  control,
  onChange,
}: {
  row: SettingRow;
  control: Extract<SettingControl, { kind: "text" | "number" }>;
  onChange: SettingsDialogProps["onChange"];
}) {
  const { value } = control;
  const [draft, setDraft] = useState(String(value));

  useEffect(() => {
    setDraft(String(value));
  }, [value]);

  function commit() {
    if (row.disabled) return;
    if (control.kind === "text") {
      if (draft !== value) onChange(row.id, draft);
      return;
    }
    const number = Number(draft);
    if (draft.trim() === "" || !Number.isFinite(number)) {
      setDraft(String(value));
      return;
    }
    const next = Math.max(control.min, Math.min(control.max, Math.round(number)));
    setDraft(String(next));
    if (next !== value) onChange(row.id, next);
  }

  return (
    <input
      type={control.kind}
      aria-label={row.name}
      disabled={row.disabled}
      placeholder={control.kind === "text" ? control.placeholder : undefined}
      min={control.kind === "number" ? control.min : undefined}
      max={control.kind === "number" ? control.max : undefined}
      spellCheck={false}
      value={draft}
      className={`h-7 rounded-md border border-separator bg-window px-2 focus:border-accent focus:outline-none disabled:opacity-50 ${control.kind === "number" ? "w-20" : "w-[200px]"}`}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          commit();
        }
      }}
    />
  );
}
