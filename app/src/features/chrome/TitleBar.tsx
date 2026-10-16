// The title bar: client-side window chrome, from frostmail's toolbar
// (ADR-0011). One strip that drags the window; double-clicking its empty
// space toggles maximize (Tauri's data-tauri-drag-region). It carries the
// sidebar toggle, the window's title and our own window controls; the tab
// bar takes the title's place when notes open (M3). Containers decide what
// each command does.
import { Copy, Minus, PanelLeft, Square, X } from "lucide-react";
import type { MouseEvent, ReactNode } from "react";

/** WindowCommand is what a title bar control asks for. */
export type WindowCommand =
  | { kind: "toggleSidebar" }
  | { kind: "minimize" }
  | { kind: "toggleMaximize" }
  | { kind: "close" };

/** TitleBarProps are the title bar's inputs. */
export interface TitleBarProps {
  title: string;
  /** The sidebar's width, 0 while it is hidden; the toggle's segment matches it. */
  sidebarWidth: number;
  maximized: boolean;
  /** Draw minimize, maximize and close (the desktop app); a browser has its own. */
  windowControls: boolean;
  onCommand: (cmd: WindowCommand) => void;
}

const BUTTON_CLASS =
  "flex size-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";

interface TitleBarButtonProps {
  label: string;
  shortcut?: string;
  icon: ReactNode;
  pressed?: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}

function TitleBarButton({ label, shortcut, icon, pressed, onClick }: TitleBarButtonProps) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={pressed}
      title={shortcut ? `${label} (${shortcut})` : label}
      onClick={onClick}
      className={BUTTON_CLASS}
    >
      {icon}
    </button>
  );
}

/** TitleBar is the window's top strip. */
export function TitleBar(props: TitleBarProps) {
  const { onCommand } = props;
  return (
    <div
      role="toolbar"
      aria-label="Title bar"
      data-tauri-drag-region
      className="flex h-11 shrink-0 items-center border-b border-separator bg-toolbar"
    >
      <div
        data-tauri-drag-region
        className="flex h-full shrink-0 items-center gap-1 px-2"
        style={props.sidebarWidth > 0 ? { width: `${props.sidebarWidth + 1}px` } : undefined}
      >
        <TitleBarButton
          label="Toggle Sidebar"
          icon={<PanelLeft size={16} />}
          pressed={props.sidebarWidth > 0}
          onClick={() => onCommand({ kind: "toggleSidebar" })}
        />
      </div>
      <span data-tauri-drag-region className="text-toolbar-title min-w-0 flex-1 truncate px-3">
        {props.title}
      </span>
      {props.windowControls && (
        <div className="flex shrink-0 items-center gap-1 px-2">
          <TitleBarButton label="Minimize" icon={<Minus size={16} />} onClick={() => onCommand({ kind: "minimize" })} />
          <TitleBarButton
            label={props.maximized ? "Restore" : "Maximize"}
            icon={props.maximized ? <Copy size={16} /> : <Square size={16} />}
            onClick={() => onCommand({ kind: "toggleMaximize" })}
          />
          <TitleBarButton label="Close" icon={<X size={16} />} onClick={() => onCommand({ kind: "close" })} />
        </div>
      )}
    </div>
  );
}
