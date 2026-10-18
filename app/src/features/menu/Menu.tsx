// A menu (docs/specs/ui.md#menus): context menus and button menus, at a
// point in the window, with items, separators and submenus. Task T-0047
// replaces this stub.
import type { ReactNode } from "react";

/** MenuItem is an entry of a menu. */
export type MenuItem =
  | {
      kind: "item";
      /** What onChoose receives. */
      id: string;
      label: string;
      /** A 16px Lucide icon. */
      icon?: ReactNode;
      /** The keys that run it, as formatHotkey writes them ("Ctrl + W"). */
      keys?: string;
      /** A checkable item: checked or not; a plain item when absent. */
      checked?: boolean;
      disabled?: boolean;
    }
  | { kind: "separator" }
  | { kind: "submenu"; label: string; icon?: ReactNode; items: MenuItem[] };

/** MenuProps are a menu's inputs. */
export interface MenuProps {
  /** The menu's accessible name. */
  label: string;
  items: MenuItem[];
  /** Where the menu's top left corner goes, in client coordinates. */
  x: number;
  y: number;
  /** An item was chosen; the menu closes after it. */
  onChoose: (id: string) => void;
  onClose: () => void;
}

/** Menu draws a menu in a portal on the document's body. */
export function Menu(_props: MenuProps) {
  return null;
}
