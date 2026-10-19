// A menu (docs/specs/ui.md#menus): context menus and button menus, at a
// point in the window, with items, separators and nested submenus navigated
// by keyboard or pointer. Containers supply the actions.
import { Check, ChevronRight } from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

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
  /**
   * A sheet from the bottom, the whole width, items 44 high, as a narrow
   * window draws menus (docs/specs/ui.md#narrow-windows); x and y are then
   * not used.
   */
  sheet?: boolean;
}

/** Menu draws a menu in a portal on the document's body. */
export function Menu(props: MenuProps) {
  const portal = useRef<HTMLDivElement>(null);
  const { onClose } = props;
  // Focus goes back to what had it when the menu opened.
  useLayoutEffect(() => {
    const before = document.activeElement;
    return () => {
      if (before instanceof HTMLElement && before.isConnected) before.focus();
    };
  }, []);
  useEffect(() => {
    function outside(event: PointerEvent) {
      if (event.target instanceof Node && !portal.current?.contains(event.target)) onClose();
    }
    document.addEventListener("pointerdown", outside, true);
    return () => document.removeEventListener("pointerdown", outside, true);
  }, [onClose]);

  return createPortal(
    <div ref={portal}>
      <MenuList {...props} />
    </div>,
    document.body,
  );
}

interface MenuListProps extends MenuProps {
  onBack?: () => void;
  menuRef?: RefObject<HTMLDivElement | null>;
}

function MenuList({ label, items, x, y, onChoose, onClose, onBack, menuRef, sheet }: MenuListProps) {
  const ownList = useRef<HTMLDivElement>(null);
  const list = menuRef ?? ownList;
  const childList = useRef<HTMLDivElement>(null);
  const [highlight, setHighlight] = useState<number | null>(null);
  const [submenu, setSubmenu] = useState<{ index: number; x: number; y: number } | null>(null);
  const [position, setPosition] = useState({ x, y });

  useLayoutEffect(() => {
    const rect = list.current?.getBoundingClientRect();
    if (!rect || sheet) return;
    setPosition({
      x: Math.max(8, Math.min(x, window.innerWidth - rect.width - 8)),
      y: Math.max(8, Math.min(y, window.innerHeight - rect.height - 8)),
    });
  }, [x, y, list, sheet]);

  useLayoutEffect(() => {
    list.current?.focus();
  }, [list]);

  function element(index: number) {
    const child = list.current?.children[index];
    return child instanceof HTMLElement ? child : null;
  }

  function focus(index: number) {
    setHighlight(index);
    element(index)?.focus();
  }

  function open(index: number) {
    const rect = element(index)?.getBoundingClientRect();
    if (!rect) return;
    focus(index);
    setSubmenu({ index, x: rect.right, y: rect.top - 4 });
    if (submenu?.index === index) childList.current?.focus();
  }

  function choose(index: number) {
    const item = items[index];
    if (item?.kind === "submenu") open(index);
    else if (item?.kind === "item" && !item.disabled) {
      onChoose(item.id);
      onClose();
    }
  }

  function move(direction: number) {
    let index = highlight ?? (direction === 1 ? -1 : 0);
    for (let count = 0; count < items.length; count++) {
      index = (index + direction + items.length) % items.length;
      const item = items[index];
      if (item?.kind === "submenu" || (item?.kind === "item" && !item.disabled)) {
        focus(index);
        return;
      }
    }
  }

  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    event.stopPropagation();
    if (!["ArrowDown", "ArrowUp", "Enter", " ", "ArrowRight", "ArrowLeft", "Escape", "Tab"].includes(event.key)) return;
    event.preventDefault();
    if (submenu) return;
    switch (event.key) {
      case "ArrowDown":
        move(1);
        break;
      case "ArrowUp":
        move(-1);
        break;
      case "Enter":
      case " ":
        if (highlight !== null) choose(highlight);
        break;
      case "ArrowRight":
        if (highlight !== null && items[highlight]?.kind === "submenu") open(highlight);
        break;
      case "ArrowLeft":
        onBack?.();
        break;
      case "Escape":
        (onBack ?? onClose)();
        break;
      case "Tab":
        onClose();
        break;
    }
  }

  function pointerEnter(index: number) {
    const item = items[index];
    if (!item || item.kind === "separator" || (item.kind === "item" && item.disabled)) return;
    if (submenu?.index === index) return;
    setSubmenu(null);
    if (item.kind === "submenu") open(index);
    else focus(index);
  }

  const child = submenu ? items[submenu.index] : undefined;
  return (
    <>
      <div
        ref={list}
        role="menu"
        aria-label={label}
        tabIndex={-1}
        className={
          sheet
            ? "fixed inset-x-0 bottom-0 z-50 max-h-[70vh] overflow-y-auto rounded-t-xl border-t border-separator bg-window p-2 shadow-popover"
            : "fixed z-50 min-w-[180px] rounded-lg border border-separator bg-window p-1 shadow-popover"
        }
        style={
          sheet
            ? { paddingBottom: "calc(0.5rem + env(safe-area-inset-bottom))" }
            : { left: position.x, top: position.y }
        }
        onKeyDown={keyDown}
        onContextMenu={(event) => event.preventDefault()}
      >
        {items.map((item, index) => {
          if (item.kind === "separator") {
            // biome-ignore lint/suspicious/noArrayIndexKey: a separator is known by its place alone
            return <hr key={`separator-${index}`} className="my-1 h-px border-0 bg-separator" />;
          }
          const highlighted = highlight === index;
          const common = {
            "aria-label": item.label,
            className: `text-sidebar-row flex ${sheet ? "h-11" : "h-7 pointer-coarse:h-11"} items-center gap-2 rounded px-2 ${highlighted ? "bg-accent text-accent-contrast" : ""} ${item.kind === "item" && item.disabled ? "opacity-40" : ""}`,
          };
          const contents = <ItemContents item={item} highlighted={highlighted} />;
          if (item.kind === "submenu") {
            return (
              <div
                key={`submenu-${item.label}`}
                role="menuitem"
                tabIndex={-1}
                onKeyDown={keyDown}
                onPointerEnter={() => pointerEnter(index)}
                onClick={() => choose(index)}
                aria-haspopup="menu"
                aria-expanded={submenu?.index === index}
                {...common}
              >
                {contents}
              </div>
            );
          }
          return (
            // biome-ignore lint/a11y/useAriaPropsSupportedByRole lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: Biome cannot resolve the conditional menu item role; the list handles keyboard events.
            <div
              key={item.id}
              role={item.checked === undefined ? "menuitem" : "menuitemcheckbox"}
              tabIndex={-1}
              onPointerEnter={() => pointerEnter(index)}
              onClick={() => choose(index)}
              aria-checked={item.checked}
              aria-disabled={item.disabled || undefined}
              {...common}
            >
              {contents}
            </div>
          );
        })}
      </div>
      {submenu && child?.kind === "submenu" && (
        <MenuList
          key={submenu.index}
          menuRef={childList}
          label={child.label}
          items={child.items}
          x={submenu.x}
          y={submenu.y}
          onChoose={onChoose}
          onClose={onClose}
          sheet={sheet}
          onBack={() => {
            setSubmenu(null);
            focus(submenu.index);
          }}
        />
      )}
    </>
  );
}

function ItemContents({ item, highlighted }: { item: Exclude<MenuItem, { kind: "separator" }>; highlighted: boolean }) {
  const checkable = item.kind === "item" && item.checked !== undefined;
  return (
    <>
      <span className={`w-4 shrink-0 ${highlighted ? "" : "text-secondary"}`} aria-hidden>
        {checkable ? item.checked && <Check size={14} /> : item.icon}
      </span>
      <span className="min-w-0 flex-1 truncate">{item.label}</span>
      {item.kind === "item" && item.keys && (
        <span className={`ml-4 shrink-0 text-xs leading-4 ${highlighted ? "" : "text-tertiary"}`}>{item.keys}</span>
      )}
      {item.kind === "submenu" && (
        <ChevronRight size={14} className={`shrink-0 ${highlighted ? "" : "text-secondary"}`} aria-hidden />
      )}
    </>
  );
}
