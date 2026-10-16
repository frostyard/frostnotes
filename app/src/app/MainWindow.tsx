// The main window's container (ADR-0011): it connects the title bar, the
// sidebar, the workspace placeholder and the status bar to the stores.
import { isTauri } from "@tauri-apps/api/core";
import type { ReactNode } from "react";

import { useNotes, useUI } from "../data/stores";
import { TitleBar } from "../features/chrome/TitleBar";
import { StatusBar } from "../features/status/StatusBar";
import { VaultSidebar } from "../features/workspace/VaultSidebar";
import { Welcome } from "../features/workspace/Welcome";
import { useMaximized, windowCommand } from "./chrome";

const SIDEBAR_WIDTH = 240;

/** Frame is the window: the title bar above body, the status bar below. */
function Frame({ sidebar, children }: { sidebar: boolean; children: ReactNode }) {
  const maximized = useMaximized();
  const { connection, hello, vaults } = useNotes();
  return (
    <div className="flex h-full flex-col bg-window">
      <TitleBar
        title="Frostnotes"
        sidebarWidth={sidebar ? SIDEBAR_WIDTH : 0}
        maximized={maximized}
        windowControls={isTauri()}
        onCommand={windowCommand}
      />
      <div className="flex min-h-0 flex-1">{children}</div>
      <StatusBar connection={connection} hello={hello} vaultCount={vaults.length} />
    </div>
  );
}

/** MainWindow shows notesd's hello and vaults, kept live by Session. */
export function MainWindow() {
  const sidebarVisible = useUI((s) => s.sidebarVisible);
  const { hello, vaults } = useNotes();
  return (
    <Frame sidebar={sidebarVisible}>
      {sidebarVisible && <VaultSidebar width={SIDEBAR_WIDTH} vaults={vaults} />}
      {hello && <Welcome hello={hello} vaults={vaults} />}
    </Frame>
  );
}

/** Connecting stands in for MainWindow until notesd answers; the title bar stays usable. */
export function Connecting() {
  const connection = useNotes((s) => s.connection);
  const text = connection.state === "lost" ? `Reconnecting to notesd… (${connection.reason})` : "Connecting to notesd…";
  return (
    <Frame sidebar={false}>
      <div data-tauri-drag-region className="text-empty flex flex-1 items-center justify-center px-8 text-secondary">
        {text}
      </div>
    </Frame>
  );
}
