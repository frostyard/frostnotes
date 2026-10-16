// The app root: the connection to notesd around the main window.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect } from "react";

import { type Connect, Session } from "../data/session";
import { Connecting, MainWindow } from "./MainWindow";

/** App connects to notesd with connect and shows the main window. */
export function App(props: { connect: Connect }) {
  // Tauri creates the window hidden; show it now that it has rendered, so
  // it never flashes WebKit's blank white page (src-tauri main.rs).
  useEffect(() => {
    if (isTauri()) void getCurrentWindow().show();
  }, []);
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      <MainWindow />
    </Session>
  );
}
