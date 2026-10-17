// A tab group's strip (docs/specs/ui.md, Tab strip): its tabs, the New tab
// and Tab list buttons, and the commands they send. The workspace's
// containers decide what each command does, open the menus, and handle
// drops. Pointer and keyboard input send commands; keyboard activation
// moves focus once the requested tab is rendered active.
import { ChevronDown, Pin, Plus, X } from "lucide-react";
import { type DragEvent, type KeyboardEvent, useEffect, useRef } from "react";

/** TabItem is a tab as its strip shows it. */
export interface TabItem {
  /** Stable for the tab's life. */
  id: string;
  /** The note's title, or the view's ("New tab", "History of Garden plan"). */
  title: string;
  /** The group's active tab; exactly one tab of a group is. */
  active: boolean;
  pinned: boolean;
}

/** TabCommand is what the strip asks of the workspace. */
export type TabCommand =
  | { kind: "activate"; id: string }
  | { kind: "close"; id: string }
  | { kind: "newTab" }
  /** Open a tab's menu at (x, y), in client coordinates. */
  | { kind: "menu"; id: string; x: number; y: number }
  /** Open the tab list at (x, y), in client coordinates. */
  | { kind: "list"; x: number; y: number };

/** TabStripProps are a strip's inputs. */
export interface TabStripProps {
  /** The tab list's accessible name. */
  label: string;
  tabs: TabItem[];
  /** This group holds the window's active tab. */
  focused: boolean;
  /** The strip sits in the title bar row, so its empty space drags the window. */
  dragRegion: boolean;
  onCommand: (command: TabCommand) => void;
  /** A tab started to drag; the workspace's drag and drop takes it from there. */
  onDragStart: (id: string, event: DragEvent<HTMLElement>) => void;
}

/** TabStrip draws a tab group's strip. */
export function TabStrip(props: TabStripProps) {
  const { tabs, onCommand } = props;
  const pendingFocus = useRef<string | null>(null);
  const tabElements = useRef(new Map<string, HTMLDivElement>());

  useEffect(() => {
    const target = tabs.find((tab) => tab.id === pendingFocus.current);
    if (target?.active) {
      tabElements.current.get(target.id)?.focus();
      pendingFocus.current = null;
    } else if (!target) {
      pendingFocus.current = null;
    }
  }, [tabs]);

  function onKeyDown(tab: TabItem, index: number, event: KeyboardEvent<HTMLDivElement>) {
    let nextIndex: number | undefined;
    switch (event.key) {
      case "ArrowLeft":
        nextIndex = index - 1;
        break;
      case "ArrowRight":
        nextIndex = index + 1;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = tabs.length - 1;
        break;
    }
    if (nextIndex !== undefined) {
      event.preventDefault();
      event.stopPropagation();
      const next = tabs[nextIndex];
      if (next && next.id !== tab.id) {
        pendingFocus.current = next.id;
        onCommand({ kind: "activate", id: next.id });
      }
    } else if (event.key === "Delete" && !tab.pinned) {
      onCommand({ kind: "close", id: tab.id });
    } else if (event.key === "ContextMenu" || (event.key === "F10" && event.shiftKey)) {
      event.preventDefault();
      const { left, bottom } = event.currentTarget.getBoundingClientRect();
      onCommand({ kind: "menu", id: tab.id, x: left, y: bottom });
    }
  }

  return (
    <div
      role="tablist"
      aria-label={props.label}
      data-tauri-drag-region={props.dragRegion || undefined}
      className="flex h-11 items-center gap-0.5 bg-toolbar px-2"
    >
      {tabs.map((tab, index) => (
        <div
          key={tab.id}
          ref={(element) => {
            if (element) tabElements.current.set(tab.id, element);
            else tabElements.current.delete(tab.id);
          }}
          role="tab"
          aria-label={tab.title}
          aria-selected={tab.active}
          tabIndex={tab.active ? 0 : -1}
          title={tab.title}
          data-tab-id={tab.id}
          draggable
          className={`group flex h-[30px] min-w-[80px] max-w-[220px] flex-[0_1_220px] items-center gap-1.5 rounded-md px-2.5 text-tab ${
            tab.active
              ? `bg-window text-primary${props.focused ? " font-medium" : ""}`
              : "text-secondary hover:bg-selection-inactive"
          }`}
          onClick={() => onCommand({ kind: "activate", id: tab.id })}
          onAuxClick={(event) => {
            if (event.button === 1 && !tab.pinned) onCommand({ kind: "close", id: tab.id });
          }}
          onContextMenu={(event) => {
            event.preventDefault();
            onCommand({ kind: "menu", id: tab.id, x: event.clientX, y: event.clientY });
          }}
          onDragStart={(event) => props.onDragStart(tab.id, event)}
          onKeyDown={(event) => onKeyDown(tab, index, event)}
        >
          <span className="min-w-0 flex-1 truncate">{tab.title}</span>
          {tab.pinned ? (
            <Pin size={12} role="img" aria-label="Pinned" aria-hidden={false} className="shrink-0" />
          ) : (
            <button
              type="button"
              aria-label={`Close ${tab.title}`}
              tabIndex={-1}
              className={`flex size-4 shrink-0 items-center justify-center rounded ${
                tab.active ? "opacity-100" : "opacity-0 group-hover:opacity-100"
              }`}
              onClick={(event) => {
                event.stopPropagation();
                onCommand({ kind: "close", id: tab.id });
              }}
            >
              <X size={12} />
            </button>
          )}
        </div>
      ))}
      <button
        type="button"
        aria-label="New tab"
        title="New tab (Ctrl + T)"
        className={BUTTON_CLASS}
        onClick={() => onCommand({ kind: "newTab" })}
      >
        <Plus size={16} />
      </button>
      <div data-tauri-drag-region={props.dragRegion || undefined} className="h-full flex-1" />
      <button
        type="button"
        aria-label="Tab list"
        aria-haspopup="menu"
        title="Tab list"
        className={BUTTON_CLASS}
        onClick={(event) => {
          const { left, bottom } = event.currentTarget.getBoundingClientRect();
          onCommand({ kind: "list", x: left, y: bottom });
        }}
      >
        <ChevronDown size={16} />
      </button>
    </div>
  );
}

const BUTTON_CLASS =
  "flex size-7 shrink-0 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive";
