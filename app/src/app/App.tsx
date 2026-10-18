// The app root: the connection to notesd around the main window.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect } from "react";

import { feed } from "../data/events";
import { type Connect, Session } from "../data/session";
import { useNotes } from "../data/stores";
import { wireWorkspace } from "../workspace/store";
import { Connecting, MainWindow } from "./MainWindow";

/** App connects to notesd with connect and shows the main window. */
export function App(props: { connect: Connect }) {
  // Tauri creates the window hidden; show it now that it has rendered, so
  // it never flashes WebKit's blank white page (src-tauri main.rs).
  useEffect(() => {
    if (isTauri()) void getCurrentWindow().show();
  }, []);
  // The workspace follows the window's vault and the event feed; this runs
  // before Session's first event arrives, so replays reach it.
  useEffect(() => wireWorkspace(feed, useNotes), []);
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      <MainWindow />
    </Session>
  );
}
