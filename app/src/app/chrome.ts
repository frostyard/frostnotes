// The window behind the title bar: Tauri's window API on the desktop, nothing
// in a browser (ADR-0011).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect, useState } from "react";

import { useUI } from "../data/stores";
import type { WindowCommand } from "../features/chrome/TitleBar";

/** useMaximized follows the window's maximized state; false in a browser. */
export function useMaximized(): boolean {
  const [maximized, setMaximized] = useState(false);
  useEffect(() => {
    if (!isTauri()) return;
    const win = getCurrentWindow();
    const update = () => void win.isMaximized().then(setMaximized);
    update();
    const off = win.onResized(update);
    return () => void off.then((stop) => stop());
  }, []);
  return maximized;
}

/** windowCommand carries out a title bar command. */
export function windowCommand(command: WindowCommand): void {
  switch (command.kind) {
    case "toggleSidebar":
      useUI.getState().toggleSidebar();
      return;
    case "minimize":
      if (isTauri()) void getCurrentWindow().minimize();
      return;
    case "toggleMaximize":
      if (isTauri()) void getCurrentWindow().toggleMaximize();
      return;
    case "close":
      if (isTauri()) void getCurrentWindow().close();
      return;
  }
}
