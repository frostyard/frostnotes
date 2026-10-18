// The main window's container (docs/specs/ui.md#layout): the ribbon, the
// sidebars and the workspace side by side, each column drawing its part of
// the title bar row, and the status bar under them. It holds the window's
// command registry and its one key listener.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { CalendarDays, FileSearch, Files, PanelLeft, Terminal } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useSyncExternalStore } from "react";

import { installKeyListener } from "../commands/keys";
import { CommandRegistry, CommandsContext } from "../commands/registry";
import { feed } from "../data/events";
import { ask } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useClient, useOpenVault } from "../data/session";
import { useNotes } from "../data/stores";
import { useVaultState } from "../data/vaultState";
import { DocumentRegistry, DocumentsContext } from "../editor/registry";
import { IconButton } from "../features/chrome/IconButton";
import { Ribbon } from "../features/chrome/Ribbon";
import { TitleBar } from "../features/chrome/TitleBar";
import { WindowControls } from "../features/chrome/WindowControls";
import { StatusBar } from "../features/status/StatusBar";
import { PaneTabs } from "../features/workspace/PaneTabs";
import { Splitter } from "../features/workspace/Splitter";
import { VaultRow } from "../features/workspace/VaultRow";
import { formatHotkey, type HotkeyCommand, hotkeysFor } from "../lib/keymap";
import { type PaneId, SIDEBAR_MIN, type Side } from "../workspace/model";
import { activeTab, selectPane, setSidebarWidth } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { useMaximized, windowCommand } from "./chrome";
import { editorCommands, workspaceCommands } from "./commands";
import { openConflictsMenu, openEditingModeMenu } from "./menus";
import { Overlays } from "./Overlays";
import { PANE_INFO, Pane } from "./Panes";
import { statusSync, useBacklinkCount } from "./status";
import { WorkspaceView } from "./WorkspaceView";

/** RIBBON is the ribbon's width; WORKSPACE_MIN the least the workspace keeps (docs/specs/ui.md#layout). */
const RIBBON = 44;
const WORKSPACE_MIN = 320;

const keysOf = (command: string) => {
  const k = hotkeysFor(command as HotkeyCommand)[0];
  return k ? formatHotkey(k) : undefined;
};

function useWindowWidth(): number {
  return useSyncExternalStore(
    (onChange) => {
      window.addEventListener("resize", onChange);
      return () => window.removeEventListener("resize", onChange);
    },
    () => window.innerWidth,
  );
}

/**
 * fitSidebars gives the widths the window draws the sidebars at, 0 for one
 * it does not draw: open sidebars give way to WORKSPACE_MIN, the right
 * first, down to SIDEBAR_MIN; a window too narrow for them at that
 * collapses the right sidebar, then the left. The workspace keeps their
 * widths and open flags, for a wider window.
 */
export function fitSidebars(
  width: number,
  left: { open: boolean; width: number },
  right: { open: boolean; width: number },
): { left: number; right: number } {
  const room = width - RIBBON - WORKSPACE_MIN;
  const squeeze = (l: number, r: number): { left: number; right: number } | null => {
    let over = l + r - room;
    const giveR = Math.min(Math.max(0, over), r > 0 ? r - SIDEBAR_MIN : 0);
    r -= giveR;
    over -= giveR;
    const giveL = Math.min(Math.max(0, over), l > 0 ? l - SIDEBAR_MIN : 0);
    l -= giveL;
    over -= giveL;
    return over <= 0 ? { left: l, right: r } : null;
  };
  const l = left.open ? left.width : 0;
  const r = right.open ? right.width : 0;
  return squeeze(l, r) ?? squeeze(l, 0) ?? { left: 0, right: 0 };
}

/** MainWindow is the window once notesd has answered. */
export function MainWindow() {
  const client = useClient();
  const registry = useMemo(() => new CommandRegistry(), []);
  // The documents outlive a reconnect: the client they write with is read when they write.
  const clientRef = useRef(client);
  clientRef.current = client;
  const docs = useMemo(() => new DocumentRegistry(() => clientRef.current), []);
  useEffect(() => registry.register(...workspaceCommands(client), ...editorCommands(docs)), [registry, client, docs]);
  // The chrome asks which commands apply: draw it again as commands come and go.
  useSyncExternalStore(registry.subscribe, registry.version);
  useEffect(() => installKeyListener(registry), [registry]);
  useEffect(() => feed.listen((e) => docs.event(e)), [docs]);
  // A new client is a reconnect: catch every document up.
  const firstClient = useRef(client);
  useEffect(() => {
    if (client !== firstClient.current) docs.reconnected();
  }, [client, docs]);
  useEffect(() => keepUnsaved(docs), [docs]);

  const ws = useWorkspace((s) => s.ws);
  const update = useWorkspace((s) => s.update);
  const showRibbon = usePrefs((s) => s.showRibbon);
  const { connection, vaults, vaultId } = useNotes();
  const openVault = useOpenVault();
  const maximized = useMaximized();
  const sync = useVaultState((s) => s.sync);
  const editingMode = usePrefs((s) => s.editingMode);
  const active = activeTab(ws);
  const activeNote = active.view.type === "note" ? active.view : null;
  const backlinks = useBacklinkCount(activeNote?.path ?? null);
  const widths = fitSidebars(useWindowWidth(), ws.left, ws.right);
  const shown = { left: widths.left > 0, right: widths.right > 0 };
  const controls = isTauri();

  const windowControls = (
    <WindowControls
      rightOpen={shown.right}
      maximized={maximized}
      windowControls={controls}
      onToggleRight={() => registry.run("app:toggle-right-sidebar")}
      onWindow={windowCommand}
    />
  );
  const sidebar = (side: Side) => {
    const bar = ws[side];
    const selected = bar.panes[bar.active] ?? (bar.panes[0] as PaneId);
    let start = bar.width;
    return (
      <>
        {side === "right" && (
          <Splitter
            orientation="vertical"
            label="Resize right sidebar"
            value={(100 * bar.width) / window.innerWidth}
            onStart={() => {
              start = useWorkspace.getState().ws.right.width;
            }}
            onMove={(d) => update((w) => setSidebarWidth(w, "right", start - d))}
          />
        )}
        <aside
          aria-label={side === "left" ? "Left sidebar" : "Right sidebar"}
          className="flex shrink-0 flex-col bg-sidebar"
          style={{ width: widths[side] }}
        >
          <div data-tauri-drag-region className="flex shrink-0 items-center border-b border-separator bg-toolbar">
            <PaneTabs
              label={side === "left" ? "Left sidebar panes" : "Right sidebar panes"}
              panes={bar.panes.map((id) => ({ id, ...PANE_INFO[id] }))}
              selected={selected}
              onSelect={(id) => update((w) => selectPane(w, side, id as PaneId))}
            />
            <div data-tauri-drag-region className="h-11 flex-1" />
            {side === "right" && windowControls}
          </div>
          <Pane id={selected} />
          {side === "left" && (
            <VaultRow
              vaults={vaults}
              current={vaultId}
              settingsDisabled={!registry.applies("app:open-settings")}
              onChoose={(id) => void openVault(id)}
              onSettings={() => registry.run("app:open-settings")}
            />
          )}
        </aside>
        {side === "left" && (
          <Splitter
            orientation="vertical"
            label="Resize left sidebar"
            value={(100 * bar.width) / window.innerWidth}
            onStart={() => {
              start = useWorkspace.getState().ws.left.width;
            }}
            onMove={(d) => update((w) => setSidebarWidth(w, "left", start + d))}
          />
        )}
      </>
    );
  };

  const ribbonItem = (command: string, label: string, icon: ReactNode) => ({
    command,
    label,
    icon,
    keys: keysOf(command),
    disabled: !registry.applies(command),
  });
  return (
    <CommandsContext.Provider value={registry}>
      <DocumentsContext.Provider value={docs}>
        <div className="flex h-full flex-col bg-window">
          <div className="relative flex min-h-0 flex-1">
            <div className="flex shrink-0 flex-col border-r border-separator bg-sidebar" style={{ width: RIBBON }}>
              <div data-tauri-drag-region className="flex shrink-0 justify-center border-b border-separator bg-toolbar">
                <div className="flex h-11 items-center">
                  <IconButton
                    label="Toggle left sidebar"
                    icon={<PanelLeft size={16} />}
                    pressed={shown.left}
                    onClick={() => registry.run("app:toggle-left-sidebar")}
                  />
                </div>
              </div>
              {showRibbon && (
                <Ribbon
                  items={[
                    ribbonItem("switcher:open", "Open quick switcher", <FileSearch size={16} />),
                    ribbonItem("daily-notes", "Open today's daily note", <CalendarDays size={16} />),
                    ribbonItem("insert-template", "Insert template", <Files size={16} />),
                    ribbonItem("command-palette:open", "Open command palette", <Terminal size={16} />),
                  ]}
                  onRun={(c) => registry.run(c)}
                />
              )}
            </div>
            {shown.left && sidebar("left")}
            {vaultId === null ? <NoVault /> : <WorkspaceView reserve={shown.right ? 0 : controls ? 140 : 44} />}
            {shown.right && sidebar("right")}
            {!shown.right && <div className="absolute top-0 right-0">{windowControls}</div>}
          </div>
          <StatusBar
            connection={connection}
            sync={sync && statusSync(sync)}
            backlinks={activeNote ? backlinks : null}
            counts={null}
            mode={activeNote ? (activeNote.mode === "read" ? "read" : editingMode) : null}
            onConflicts={() => {
              if (vaultId === null) return;
              void client.sync
                .conflicts({ vaultId })
                .then((list) => openConflictsMenu(list, window.innerWidth, window.innerHeight));
            }}
            onBacklinks={() => update((w) => selectPane(w, "right", "backlinks"))}
            onMode={(x, y) => openEditingModeMenu(active.id, x, y)}
          />
          <Overlays />
        </div>
      </DocumentsContext.Provider>
    </CommandsContext.Provider>
  );
}

/**
 * keepUnsaved saves every document when the window goes, and asks before
 * a window with unsaved text closes (docs/design/app.md#operational-notes).
 */
function keepUnsaved(docs: DocumentRegistry): () => void {
  const beforeUnload = (e: BeforeUnloadEvent) => {
    if (!docs.unsaved()) return;
    void docs.flushAll();
    e.preventDefault();
  };
  window.addEventListener("beforeunload", beforeUnload);
  let offClose: (() => void) | undefined;
  if (isTauri()) {
    void getCurrentWindow()
      .onCloseRequested(async (e) => {
        if (!docs.unsaved()) return;
        e.preventDefault();
        await docs.flushAll();
        const close =
          !docs.unsaved() ||
          (await ask({
            title: "Close anyway?",
            text: "Some notes are not saved yet; what was typed in them is lost.",
            confirmLabel: "Close",
            danger: true,
          }));
        if (close) void getCurrentWindow().destroy();
      })
      .then((off) => {
        offClose = off;
      });
  }
  return () => {
    window.removeEventListener("beforeunload", beforeUnload);
    offClose?.();
  };
}

function NoVault() {
  return (
    <main aria-label="Workspace" className="flex min-w-0 flex-1 flex-col bg-window">
      <div data-tauri-drag-region className="h-11 shrink-0 border-b border-separator bg-toolbar" />
      <p className="text-empty m-auto max-w-md px-8 text-center text-secondary">
        No vaults yet. Register one with <code className="select-text">notesctl vault add PATH</code>.
      </p>
    </main>
  );
}

/** Connecting stands in for MainWindow until notesd answers; the title bar stays usable. */
export function Connecting() {
  const connection = useNotes((s) => s.connection);
  const maximized = useMaximized();
  const text = connection.state === "lost" ? `Reconnecting to notesd… (${connection.reason})` : "Connecting to notesd…";
  return (
    <div className="flex h-full flex-col bg-window">
      <TitleBar
        title="Frostnotes"
        sidebarWidth={0}
        maximized={maximized}
        windowControls={isTauri()}
        onCommand={windowCommand}
      />
      <div data-tauri-drag-region className="text-empty flex flex-1 items-center justify-center px-8 text-secondary">
        {text}
      </div>
      <StatusBar
        connection={connection}
        sync={null}
        backlinks={null}
        counts={null}
        mode={null}
        onConflicts={() => {}}
        onBacklinks={() => {}}
        onMode={() => {}}
      />
    </div>
  );
}
