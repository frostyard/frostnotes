// The title bar row's right end (docs/specs/ui.md#title-bar-row): Toggle
// right sidebar, then the window controls on the desktop; a browser draws
// its own.
import { Copy, Minus, PanelRight, Square, X } from "lucide-react";

import { IconButton } from "./IconButton";
import type { WindowCommand } from "./TitleBar";

export interface WindowControlsProps {
  rightOpen: boolean;
  maximized: boolean;
  /** Draw minimize, maximize and close. */
  windowControls: boolean;
  onToggleRight: () => void;
  onWindow: (command: WindowCommand) => void;
}

/** WindowControls draws the title bar row's right end. */
export function WindowControls(props: WindowControlsProps) {
  return (
    <div className="flex h-11 shrink-0 items-center gap-1 px-2">
      <IconButton
        label="Toggle right sidebar"
        icon={<PanelRight size={16} />}
        pressed={props.rightOpen}
        onClick={props.onToggleRight}
      />
      {props.windowControls && (
        <>
          <IconButton
            label="Minimize"
            icon={<Minus size={16} />}
            onClick={() => props.onWindow({ kind: "minimize" })}
          />
          <IconButton
            label={props.maximized ? "Restore" : "Maximize"}
            icon={props.maximized ? <Copy size={14} /> : <Square size={14} />}
            onClick={() => props.onWindow({ kind: "toggleMaximize" })}
          />
          <IconButton label="Close" icon={<X size={16} />} onClick={() => props.onWindow({ kind: "close" })} />
        </>
      )}
    </div>
  );
}
