// The title bar row's right end (docs/specs/ui.md#title-bar-row): Toggle
// right sidebar, then the window controls on the desktop; a browser draws
// its own.

import { IconButton } from "@frostyard/lodge-ui/react";
import { Copy, Minus, PanelRight, Square, X } from "lucide-react";
import type { WindowCommand } from "./TitleBar";

export interface WindowControlsProps {
  rightOpen: boolean;
  maximized: boolean;
  /** Draw minimize, maximize and close. */
  windowControls: boolean;
  /** Toggle right sidebar; without it, a window with no sidebars, the button is not drawn. */
  onToggleRight?: () => void;
  onWindow: (command: WindowCommand) => void;
}

/** WindowControls draws the title bar row's right end. */
export function WindowControls(props: WindowControlsProps) {
  return (
    <div className="flex h-11 shrink-0 items-center gap-1 px-2">
      {props.onToggleRight && (
        <IconButton
          label="Toggle right sidebar"
          icon={PanelRight}
          pressed={props.rightOpen}
          onClick={props.onToggleRight}
        />
      )}
      {props.windowControls && (
        <>
          <IconButton label="Minimize" icon={Minus} onClick={() => props.onWindow({ kind: "minimize" })} />
          <IconButton
            label={props.maximized ? "Restore" : "Maximize"}
            icon={props.maximized ? Copy : Square}
            onClick={() => props.onWindow({ kind: "toggleMaximize" })}
          />
          <IconButton label="Close" icon={X} onClick={() => props.onWindow({ kind: "close" })} />
        </>
      )}
    </div>
  );
}
