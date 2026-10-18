// The app root: the connection to notesd around the main window.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect } from "react";

import { feed } from "../data/events";
import { usePrefs } from "../data/prefs";
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
  // Appearance (docs/specs/ui.md, Tokens and Settings): Light and Dark set
  // data-theme on the root, System leaves the system's; the font size is
  // the editor's and the reading view's body size.
  const theme = usePrefs((s) => s.theme);
  const fontSize = usePrefs((s) => s.fontSize);
  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") delete root.dataset.theme;
    else root.dataset.theme = theme;
    root.style.setProperty("--note-font-size", `${fontSize}px`);
  }, [theme, fontSize]);
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      <MainWindow />
    </Session>
  );
}
