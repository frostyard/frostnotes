// The title bar row's right end (docs/specs/ui.md#title-bar-row): Toggle
// right sidebar.

import { IconButton } from "@frostyard/lodge-ui/react";
import { PanelRight } from "lucide-react";

export interface TitleBarEndProps {
  rightOpen: boolean;
  /** Toggle right sidebar; without it, a window with no sidebars, the button is not drawn. */
  onToggleRight?: () => void;
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
    </div>
  );
}
