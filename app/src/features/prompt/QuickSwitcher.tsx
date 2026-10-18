// The quick switcher (docs/specs/ui.md, Prompts): Ctrl+O's prompt over
// the vault's notes. Task T-0051 replaces this stub.
import type { SwitcherNote } from "../../lib/switcher";
import type { PromptMods } from "./Prompt";

/** SwitcherWhere is where a note opens: in the current tab, a new tab, or to the right. */
export type SwitcherWhere = "current" | "tab" | "right";

export interface QuickSwitcherProps {
  notes: readonly SwitcherNote[];
  /** The recently opened notes' paths, newest first. */
  recent: readonly string[];
  onOpen: (path: string, where: SwitcherWhere) => void;
  /** Create a note named name (as typed, without ".md"). */
  onCreate: (name: string, where: SwitcherWhere) => void;
  onClose: () => void;
}

/** whereFor maps the modifiers of Enter or a click to where the note opens. */
export function whereFor(_mods: PromptMods): SwitcherWhere {
  return "current";
}

/** QuickSwitcher draws the quick switcher. */
export function QuickSwitcher(_props: QuickSwitcherProps) {
  return null;
}
