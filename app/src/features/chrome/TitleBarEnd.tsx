// The title bar row's right end (docs/specs/ui.md#title-bar-row): Toggle
// right sidebar, then the window controls on the desktop; a browser draws
// its own. The three window buttons are lodge-ui's WindowControls.

import { IconButton, WindowControls } from "@frostyard/lodge-ui/react";
import { PanelRight } from "lucide-react";
import type { WindowCommand } from "./TitleBar";

export interface TitleBarEndProps {
  rightOpen: boolean;
  maximized: boolean;
  /** Draw minimize, maximize and close. */
  windowControls: boolean;
  /** Toggle right sidebar; without it, a window with no sidebars, the button is not drawn. */
  onToggleRight?: () => void;
  onWindow: (command: WindowCommand) => void;
}

/** TitleBarEnd draws the title bar row's right end. */
export function TitleBarEnd(props: TitleBarEndProps) {
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
      {props.windowControls && <WindowControls maximized={props.maximized} onWindow={props.onWindow} />}
    </div>
  );
}
