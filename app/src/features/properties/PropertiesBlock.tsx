// The properties block (docs/specs/ui.md, Editor and reading view): typed
// value editors, editable keys, and callbacks for changes to frontmatter.
import { ChevronRight, Plus, Text, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { PropertyType, PropertyValue } from "../../editor/properties";
import { TYPE_ICONS } from "./PropertiesPane";

/** PropertyRow is a row of the block: a key, its type and its value. */
export interface PropertyRow {
  key: string;
  type: PropertyType;
  value: PropertyValue;
}

export interface PropertiesBlockProps {
  rows: readonly PropertyRow[];
  /** Why the frontmatter is not properties; null when it is. */
  error: string | null;
  folded: boolean;
  /** Each time it changes (but not at first), the block opens its new row. */
  addRequest?: number;
  onFold: (folded: boolean) => void;
  /** Set a key, new or not, to a value. */
  onChange: (key: string, value: PropertyValue) => void;
  onRename: (from: string, to: string) => void;
  onRemove: (key: string) => void;
}

/** items is a list value's items as text; any other value is one item. */
export function items(value: PropertyValue): string[] {
  if (Array.isArray(value)) return value.map(String);
  return value === null || value === "" ? [] : [String(value)];
}

const fieldClass = "min-w-0 w-full rounded bg-transparent px-1 text-sidebar-row";

function valueText(value: PropertyValue): string {
  return value === null ? "" : typeof value === "object" ? JSON.stringify(value) : String(value);
}

function CommitField({
  label,
  value,
  type = "text",
  onCommit,
}: {
  label: string;
  value: string;
  type?: "text" | "number";
  onCommit: (text: string) => void;
}) {
  const [draft, setDraft] = useState(value);
  const cancelled = useRef(false);
  useEffect(() => setDraft(value), [value]);
  return (
    <input
      className={fieldClass}
      aria-label={label}
      type={type}
      step={type === "number" ? "any" : undefined}
      value={draft}
      onChange={(event) => setDraft(event.currentTarget.value)}
      onFocus={() => {
        cancelled.current = false;
      }}
      onBlur={(event) => {
        const text = event.currentTarget.value;
        setDraft(value);
        if (!cancelled.current && text !== value) onCommit(text);
        cancelled.current = false;
      }}
      onKeyDown={(event) => {
        if (event.nativeEvent.isComposing) return;
        if (event.key === "Escape") {
          event.preventDefault();
          cancelled.current = true;
          setDraft(value);
          event.currentTarget.blur();
        } else if (event.key === "Enter") {
          event.preventDefault();
          event.currentTarget.blur();
        }
      }}
    />
  );
}

function ListEditor({ row, onChange }: { row: PropertyRow; onChange: PropertiesBlockProps["onChange"] }) {
  const values = items(row.value);
  const occurrences = new Map<string, number>();
  return (
    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1 py-1">
      <ul aria-label={row.key} className="contents">
        {values.map((item, index) => {
          const occurrence = occurrences.get(item) ?? 0;
          occurrences.set(item, occurrence + 1);
          return (
            <li
              key={JSON.stringify([item, occurrence])}
              className={`flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 ${row.type === "tags" ? "bg-tag text-accent" : "bg-code"}`}
            >
              {item}
              <button
                type="button"
                aria-label={`Remove ${item}`}
                onClick={() =>
                  onChange(
                    row.key,
                    values.filter((_, position) => position !== index),
                  )
                }
              >
                <X size={12} aria-hidden="true" />
              </button>
            </li>
          );
        })}
      </ul>
      <input
        className={`${fieldClass} min-w-16 flex-1`}
        aria-label={`Add to ${row.key}`}
        onKeyDown={(event) => {
          if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
          event.preventDefault();
          const text = event.currentTarget.value.trim();
          const item = row.type === "tags" ? text.replace(/^#/, "").trim() : text;
          if (item) onChange(row.key, [...values, item]);
          event.currentTarget.value = "";
        }}
      />
    </div>
  );
}

function ValueEditor({ row, onChange }: { row: PropertyRow; onChange: PropertiesBlockProps["onChange"] }) {
  switch (row.type) {
    case "multitext":
    case "aliases":
    case "tags":
      return <ListEditor row={row} onChange={onChange} />;
    case "checkbox":
      return (
        <input
          type="checkbox"
          aria-label={row.key}
          checked={Boolean(row.value)}
          onChange={(event) => onChange(row.key, event.currentTarget.checked)}
        />
      );
    case "date":
    case "datetime":
      return (
        <input
          className={fieldClass}
          type={row.type === "date" ? "date" : "datetime-local"}
          aria-label={row.key}
          value={valueText(row.value)}
          onChange={(event) => onChange(row.key, event.currentTarget.value)}
        />
      );
    default:
      return (
        <CommitField
          label={row.key}
          type={row.type === "number" ? "number" : "text"}
          value={valueText(row.value)}
          onCommit={(text) => {
            if (row.type !== "number") onChange(row.key, text);
            else if (text.trim() !== "" && Number.isFinite(Number(text))) onChange(row.key, Number(text));
          }}
        />
      );
  }
}

function PropertyFields({
  row,
  onChange,
  onRename,
  onRemove,
}: Pick<PropertiesBlockProps, "onChange" | "onRename" | "onRemove"> & { row: PropertyRow }) {
  const Icon = (Object.hasOwn(TYPE_ICONS, row.type) ? TYPE_ICONS[row.type] : undefined) ?? Text;
  return (
    // biome-ignore lint/a11y/useSemanticElements: This compact property row is a group of editors, not a fieldset with a legend.
    <div
      role="group"
      aria-label={row.key}
      data-property={row.key}
      className="group flex min-h-[30px] items-center gap-2 rounded-md px-1 hover:bg-code"
    >
      <div className="flex w-[9em] shrink-0 items-center gap-1 text-sidebar-row text-secondary">
        <Icon size={14} aria-hidden="true" data-type={row.type} className="shrink-0" />
        <CommitField
          label="Property name"
          value={row.key}
          onCommit={(text) => {
            const name = text.trim();
            if (name && name !== row.key) onRename(row.key, name);
          }}
        />
      </div>
      <div className="flex min-w-0 flex-1 items-center">
        <ValueEditor row={row} onChange={onChange} />
      </div>
      <button
        type="button"
        aria-label={`Remove ${row.key}`}
        className="shrink-0 rounded p-1 text-secondary opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
        onClick={() => onRemove(row.key)}
      >
        <X size={14} aria-hidden="true" />
      </button>
    </div>
  );
}

function NewProperty({
  rows,
  onChange,
  onClose,
}: Pick<PropertiesBlockProps, "rows" | "onChange"> & { onClose: () => void }) {
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    input.current?.focus();
  }, []);
  return (
    <input
      ref={input}
      className={`${fieldClass} h-[30px]`}
      aria-label="New property name"
      onBlur={(event) => {
        if (!event.currentTarget.value.trim()) onClose();
      }}
      onKeyDown={(event) => {
        if (event.nativeEvent.isComposing) return;
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
        } else if (event.key === "Enter") {
          event.preventDefault();
          const name = event.currentTarget.value.trim();
          if (!name || rows.some((row) => row.key === name)) return;
          onChange(name, "");
          onClose();
        }
      }}
    />
  );
}

/** PropertiesBlock draws a note's properties. */
export function PropertiesBlock({
  rows,
  error,
  folded,
  addRequest,
  onFold,
  onChange,
  onRename,
  onRemove,
}: PropertiesBlockProps) {
  const [adding, setAdding] = useState(false);
  const previousRequest = useRef(addRequest);
  useEffect(() => {
    if (Object.is(previousRequest.current, addRequest)) return;
    previousRequest.current = addRequest;
    onFold(false);
    setAdding(true);
  }, [addRequest, onFold]);
  return (
    <section aria-label="Properties">
      <button
        type="button"
        aria-expanded={!folded}
        className="flex items-center gap-1 py-1 text-tab text-secondary"
        onClick={() => onFold(!folded)}
      >
        <ChevronRight size={14} aria-hidden="true" className={folded ? "" : "rotate-90"} />
        Properties
      </button>
      {!folded &&
        (error !== null ? (
          <p className="text-sidebar-row text-secondary">The frontmatter is not properties: {error}</p>
        ) : (
          <>
            {rows.map((row) => (
              <PropertyFields key={row.key} row={row} onChange={onChange} onRename={onRename} onRemove={onRemove} />
            ))}
            {adding ? (
              <NewProperty rows={rows} onChange={onChange} onClose={() => setAdding(false)} />
            ) : (
              <button
                type="button"
                className="flex h-[30px] items-center gap-1 rounded-md px-1 text-sidebar-row text-secondary hover:bg-code"
                onClick={() => setAdding(true)}
              >
                <Plus size={14} aria-hidden="true" />
                Add property
              </button>
            )}
          </>
        ))}
    </section>
  );
}
