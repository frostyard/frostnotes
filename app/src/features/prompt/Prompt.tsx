// A prompt (docs/specs/ui.md, Prompts): the quick switcher's and the
// command palette's frame. Task T-0049 replaces this stub.
import type { ReactNode } from "react";

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
export function highlight(text: string, _ranges: readonly Range[], _from = 0): ReactNode[] {
  return [text];
}

/** Prompt draws a prompt in a portal on the document's body. */
export function Prompt<T>(_props: PromptProps<T>) {
  return null;
}
