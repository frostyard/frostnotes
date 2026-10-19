// The app root: the connection to notesd around the main window.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWebview } from "@tauri-apps/api/webview";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { type ReactNode, useEffect } from "react";

import { feed } from "../data/events";
import { usePrefs } from "../data/prefs";
import { type Connect, type Retry, Session } from "../data/session";
import { useNotes } from "../data/stores";
import { useLodge } from "../lodge/lodge";
import { wireWorkspace } from "../workspace/store";
import { Connecting, MainWindow } from "./MainWindow";

/**
 * App connects to notesd with connect and shows the main window. retry is
 * the web app's reconnecting, and children render under the connection
 * (the web app's bridge to lodge's shell).
 */
export function App(props: { connect: Connect; retry?: Retry; children?: ReactNode }) {
  // Tauri creates the window hidden; show it now that it has rendered, so
  // it never flashes WebKit's blank white page (src-tauri main.rs).
  useEffect(() => {
    if (isTauri()) void getCurrentWindow().show();
  }, []);
  // The workspace follows the window's vault and the event feed; this runs
  // before Session's first event arrives, so replays reach it.
  useEffect(() => wireWorkspace(feed, useNotes), []);
  // Appearance (docs/specs/ui.md, Tokens and Settings): Light and Dark set
  // data-theme on the root, System leaves the system's, or inside lodge
  // follows the shell's theme; the font size is the editor's and the
  // reading view's body size.
  const theme = usePrefs((s) => s.theme);
  const fontSize = usePrefs((s) => s.fontSize);
  const shell = useLodge((s) => s.scheme);
  useEffect(() => {
    const root = document.documentElement;
    const scheme = theme === "system" ? shell : theme;
    if (scheme === null) delete root.dataset.theme;
    else root.dataset.theme = scheme;
    root.style.setProperty("--note-font-size", `${fontSize}px`);
  }, [theme, fontSize, shell]);
  // The window's zoom: the webview's own in the desktop app, CSS zoom in a browser.
  const zoom = usePrefs((s) => s.zoom);
  useEffect(() => {
    if (isTauri()) void getCurrentWebview().setZoom(zoom);
    else document.documentElement.style.zoom = zoom === 1 ? "" : String(zoom);
  }, [zoom]);
  return (
    <Session connect={props.connect} retry={props.retry} fallback={<Connecting />}>
      <MainWindow />
      {props.children}
    </Session>
  );
}
