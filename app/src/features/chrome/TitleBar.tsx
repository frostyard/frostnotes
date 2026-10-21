// The title bar: client-side window chrome, from frostmail's toolbar
// (ADR-0011). One strip that drags the window; double-clicking its empty
// space toggles maximize (Tauri's data-tauri-drag-region). It carries the
// sidebar toggle, the window's title and our own window controls; the tab
// bar takes the title's place when notes open (M3). Containers decide what
// each command does.
import { IconButton } from "@frostyard/lodge-ui/react";
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
        <IconButton
          label="Toggle Sidebar"
          icon={PanelLeft}
          pressed={props.sidebarWidth > 0}
          onClick={() => onCommand({ kind: "toggleSidebar" })}
        />
      </div>
      <span data-tauri-drag-region className="text-toolbar-title min-w-0 flex-1 truncate px-3">
        {props.title}
      </span>
      {props.windowControls && (
        <div className="flex shrink-0 items-center gap-1 px-2">
          <IconButton label="Minimize" icon={Minus} onClick={() => onCommand({ kind: "minimize" })} />
          <IconButton
            label={props.maximized ? "Restore" : "Maximize"}
            icon={props.maximized ? Copy : Square}
            onClick={() => onCommand({ kind: "toggleMaximize" })}
          />
          <IconButton label="Close" icon={X} onClick={() => onCommand({ kind: "close" })} />
        </div>
      )}
    </div>
  );
}
