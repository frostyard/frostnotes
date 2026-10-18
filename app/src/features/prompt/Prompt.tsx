// A prompt (docs/specs/ui.md, Prompts): the quick switcher's and the
// command palette's shared modal frame, with a focused query field,
// keyboard and pointer selection, and an instructions footer.
import { type KeyboardEvent, type ReactNode, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";

import type { Range } from "../../lib/fuzzy";

/** PromptMods are the modifiers held when a row was chosen. */
export interface PromptMods {
  ctrl: boolean;
  alt: boolean;
  shift: boolean;
}

/** PromptInstruction is one item of the footer: "↵" "to open". */
export interface PromptInstruction {
  keys: string;
  text: string;
}

export interface PromptProps<T> {
  /** The dialog's accessible name ("Quick switcher"). */
  label: string;
  placeholder: string;
  query: string;
  onQuery: (query: string) => void;
  /** The results, best first. */
  items: readonly T[];
  /** Stable for an item while it shows. */
  itemKey: (item: T) => string;
  /** A row's content. */
  renderItem: (item: T) => ReactNode;
  /** Enter or a click: the selected or clicked item, or undefined when the list is empty. */
  onEnter: (item: T | undefined, mods: PromptMods) => void;
  onClose: () => void;
  instructions: PromptInstruction[];
  /** Shown instead of the list while it is empty. */
  emptyText?: string;
}

/** highlight draws text with the ranges (offset into it by `from`) in the accent, bold. */
export function highlight(text: string, ranges: readonly Range[], from = 0): ReactNode[] {
  const nodes: ReactNode[] = [];
  let end = 0;
  for (const [s, e] of ranges) {
    const start = Math.max(0, s - from);
    const stop = Math.min(text.length, e - from);
    if (start < end || start >= stop) continue;
    if (start > end) nodes.push(text.slice(end, start));
    nodes.push(
      <span key={start} className="font-semibold text-accent">
        {text.slice(start, stop)}
      </span>,
    );
    end = stop;
  }
  if (end < text.length) nodes.push(text.slice(end));
  return nodes;
}

/** Prompt draws a prompt in a portal on the document's body. */
export function Prompt<T>(props: PromptProps<T>) {
  const { label, placeholder, query, onQuery, items, itemKey, renderItem, onEnter, onClose, instructions, emptyText } =
    props;
  const listId = useId();
  const field = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState(0);

  useEffect(() => {
    field.current?.focus();
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: items is the trigger.
  useEffect(() => {
    setSelected(0);
  }, [items]);

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Tab") {
      event.preventDefault();
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Enter", "Escape"].includes(event.key)) return;
    event.preventDefault();
    event.stopPropagation();
    if (event.key === "Escape") {
      onClose();
    } else if (event.key === "Enter") {
      onEnter(items[selected], { ctrl: event.ctrlKey, alt: event.altKey, shift: event.shiftKey });
    } else if (items.length > 0) {
      const next = (selected + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      setSelected(next);
      document.getElementById(`${listId}-${next}`)?.scrollIntoView?.({ block: "nearest" });
    }
  }

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-backdrop"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={label}
        className="mt-[60px] flex max-h-[70vh] w-[700px] max-w-[80vw] flex-col overflow-hidden rounded-xl border border-separator bg-window shadow-popover"
      >
        <input
          ref={field}
          role="combobox"
          aria-label={label}
          aria-expanded="true"
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={items.length > 0 ? `${listId}-${selected}` : undefined}
          placeholder={placeholder}
          value={query}
          spellCheck={false}
          onChange={(event) => onQuery(event.target.value)}
          onKeyDown={onKeyDown}
          className="h-11 shrink-0 border-b border-separator bg-transparent px-4 text-empty outline-none"
        />
        <div id={listId} role="listbox" aria-label="Results" className="min-h-0 overflow-y-auto p-2">
          {items.map((item, index) => (
            // biome-ignore lint/a11y/useKeyWithClickEvents: The focused query field's keys choose rows.
            <div
              key={itemKey(item)}
              id={`${listId}-${index}`}
              role="option"
              tabIndex={-1}
              aria-selected={index === selected}
              className={`flex min-h-8 items-center rounded-md px-2 text-sidebar-row ${index === selected ? "bg-selection-inactive" : ""}`}
              onMouseMove={() => setSelected(index)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={(event) => onEnter(item, { ctrl: event.ctrlKey, alt: event.altKey, shift: event.shiftKey })}
            >
              {renderItem(item)}
            </div>
          ))}
          {items.length === 0 && emptyText !== undefined && (
            <p className="text-sidebar-row text-secondary">{emptyText}</p>
          )}
        </div>
        <div className="flex h-7 shrink-0 items-center gap-4 border-t border-separator px-4 text-status text-tertiary">
          {instructions.map(({ keys, text }) => (
            <span key={`${keys}-${text}`}>
              <span className="font-semibold">{keys}</span> {text}
            </span>
          ))}
        </div>
      </div>
    </div>,
    document.body,
  );
}
