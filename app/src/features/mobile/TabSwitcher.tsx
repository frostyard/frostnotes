// The tab switcher of a narrow window (docs/specs/ui.md#narrow-windows): a
// full-screen list of the group's tabs, each with its title and folder,
// a tap to activate it and Close at its right; New tab and Close all at
// its foot.
import { X } from "lucide-react";
import { useEffect, useRef } from "react";

import { TouchButton } from "./TouchButton";

/** SwitcherTab is one row of the switcher. */
export interface SwitcherTab {
  id: string;
  title: string;
  /** The folder of the tab's file, "" for the root or a view without one. */
  folder: string;
  active: boolean;
}

export interface TabSwitcherProps {
  tabs: readonly SwitcherTab[];
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  onNewTab: () => void;
  onCloseAll: () => void;
  onDismiss: () => void;
}

/** TabSwitcher draws the full-screen tab switcher. */
export function TabSwitcher(props: TabSwitcherProps) {
  const frame = useRef<HTMLDivElement>(null);
  const { onDismiss } = props;
  useEffect(() => frame.current?.focus(), []);
  // Escape dismisses it wherever the focus went, a closed row's included.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onDismiss();
      }
    };
    window.addEventListener("keydown", key, true);
    return () => window.removeEventListener("keydown", key, true);
  }, [onDismiss]);
  return (
    <div
      ref={frame}
      role="dialog"
      aria-label="Tabs"
      aria-modal="true"
      tabIndex={-1}
      className="fixed inset-0 z-40 flex flex-col bg-window"
      style={{ paddingTop: "env(safe-area-inset-top)", paddingBottom: "env(safe-area-inset-bottom)" }}
    >
      <div className="flex h-[52px] shrink-0 items-center border-b border-separator px-1">
        <h2 className="flex-1 px-3 text-toolbar-title">Tabs</h2>
        <TouchButton label="Done" icon={<X size={20} />} onClick={props.onDismiss} />
      </div>
      <ul aria-label="Open tabs" className="min-h-0 flex-1 overflow-y-auto p-2">
        {props.tabs.map((t) => (
          <li key={t.id} className={`flex items-center rounded-md ${t.active ? "bg-selection-inactive" : ""}`}>
            <button
              type="button"
              aria-current={t.active ? true : undefined}
              className="flex min-h-11 min-w-0 flex-1 flex-col justify-center px-3 py-1 text-left"
              onClick={() => props.onActivate(t.id)}
            >
              <span className="truncate text-sidebar-row">{t.title}</span>
              {t.folder !== "" && <span className="truncate text-xs leading-4 text-secondary">{t.folder}</span>}
            </button>
            <TouchButton label={`Close ${t.title}`} icon={<X size={18} />} onClick={() => props.onClose(t.id)} />
          </li>
        ))}
      </ul>
      <div className="flex h-[52px] shrink-0 items-center justify-between border-t border-separator px-3">
        <button type="button" className="h-11 px-3 text-sidebar-row text-accent" onClick={props.onNewTab}>
          New tab
        </button>
        <button type="button" className="h-11 px-3 text-sidebar-row text-flag-1" onClick={props.onCloseAll}>
          Close all
        </button>
      </div>
    </div>
  );
}
