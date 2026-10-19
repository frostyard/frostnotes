// LodgeBridge connects the window to lodge's shell
// (docs/design/app.md#in-a-browser): the active note's route and context
// go to the shell, the shell's navigate (and the frame's own route on load)
// opens a note, and the shell's theme reaches the Theme preference's
// System. lodge-client itself keeps the switcher's key from the app.
import { MAX_ROUTE, splitRoute } from "@frostyard/lodge-client";
import { useEffect } from "react";

import { viewOf } from "../app/commands";
import { windowTitle } from "../app/system";
import { useFiles } from "../data/files";
import { notify, openPrompt } from "../data/overlays";
import { openVault, useClient } from "../data/session";
import { useNotes } from "../data/stores";
import type { Client } from "../rpc/gen/api";
import { activeTab, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { type Lodge, useLodge } from "./lodge";
import { type NoteRoute, noteRef, noteRoute, noteTitle, parseNoteRoute } from "./routes";

/** LodgeBridge wires the window to the shell; it renders nothing. */
export function LodgeBridge(props: { lodge: Lodge }) {
  const { lodge } = props;
  const c = useClient();

  useEffect(() => {
    useLodge.setState({ lodge, scheme: lodge.theme });
    return lodge.on("theme", ({ scheme }) => useLodge.setState({ scheme }));
  }, [lodge]);

  // Nothing is replayed to late listeners: the frame's own route first.
  useEffect(() => {
    const go = (pathname: string) => {
      const r = parseNoteRoute(pathname);
      if (r) void openRoute(c, r);
    };
    go(lodge.history.location.pathname);
    return lodge.on("navigate", ({ route }) => go(splitRoute(route).pathname));
  }, [c, lodge]);

  useEffect(() => {
    let last = "";
    const report = () => {
      const vaultId = useNotes.getState().vaultId;
      const view = activeTab(useWorkspace.getState().ws).view;
      const path = view.type === "empty" ? "" : view.path;
      const vaultName = useNotes.getState().vaults.find((v) => v.id === vaultId)?.name ?? "";
      const key = `${vaultId}\u0000${path}\u0000${vaultName}`;
      if (key === last) return;
      last = key;
      if (vaultId === null) {
        lodge.route("/", "");
        lodge.context({ kind: null, ref: "", title: "" });
        return;
      }
      const title = path === "" ? "" : noteTitle(path);
      const route = noteRoute(vaultId, path);
      // A route is at most 2,048 characters (lodge-client.md, Routes); the
      // frame's title is the window's (docs/specs/ui.md#in-a-browser).
      lodge.route(
        route.length <= MAX_ROUTE ? route : noteRoute(vaultId, ""),
        windowTitle(title || null, vaultName || null),
      );
      lodge.context(
        path === "" ? { kind: null, ref: "", title: "" } : { kind: "note", ref: noteRef(vaultId, path), title },
      );
    };
    report();
    const offWorkspace = useWorkspace.subscribe(report);
    const offNotes = useNotes.subscribe(report);
    return () => {
      offWorkspace();
      offNotes();
    };
  }, [lodge]);

  return null;
}

/**
 * openRoute opens what a route names: its vault, if it is not the
 * window's, then its file. A file that is gone opens the quick switcher.
 */
export async function openRoute(c: Client, r: NoteRoute): Promise<void> {
  const notes = useNotes.getState();
  if (!notes.vaults.some((v) => v.id === r.vaultId)) {
    notify(`Vault ${r.vaultId} is not on this notesd.`);
    return;
  }
  if (notes.vaultId !== r.vaultId) await openVault(c, r.vaultId);
  if (r.path === "") return;
  const files = useFiles.getState();
  if (files.vaultId === r.vaultId && files.loaded && !files.files.has(r.path)) {
    notify(`${noteTitle(r.path)} is not in this vault.`);
    openPrompt("switcher");
    return;
  }
  const view = activeTab(useWorkspace.getState().ws).view;
  if (view.type !== "empty" && view.path === r.path) return;
  useWorkspace.getState().update((w) => open(w, viewOf(r.path), "here"));
}
