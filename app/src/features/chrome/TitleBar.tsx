// The title bar: the strip above the window while it connects, from
// frostmail's toolbar (ADR-0011). It carries the sidebar toggle and the
// window's title; the tab bar takes the title's place when notes open (M3).
import { IconButton } from "@frostyard/lodge-ui/react";
import { PanelLeft } from "lucide-react";

/** TitleBarProps are the title bar's inputs. */
export interface TitleBarProps {
  title: string;
  /** The sidebar's width, 0 while it is hidden; the toggle's segment matches it. */
  sidebarWidth: number;
  onToggleSidebar: () => void;
}

/** TitleBar is the window's top strip. */
export function TitleBar(props: TitleBarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Title bar"
      className="flex h-11 shrink-0 items-center border-b border-separator bg-toolbar"
    >
      <div
        className="flex h-full shrink-0 items-center gap-1 px-2"
        style={props.sidebarWidth > 0 ? { width: `${props.sidebarWidth + 1}px` } : undefined}
      >
        <IconButton
          label="Toggle Sidebar"
          icon={PanelLeft}
          pressed={props.sidebarWidth > 0}
          onClick={props.onToggleSidebar}
        />
      </div>
      <span className="text-toolbar-title min-w-0 flex-1 truncate px-3">{props.title}</span>
    </div>
  );
}
