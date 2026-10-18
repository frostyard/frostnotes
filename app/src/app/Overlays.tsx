// What the window draws over its layout (data/overlays.ts): the open
// menu, the quick switcher or the command palette, and the notices. The
// switcher lists the vault's notes and the workspace's recent files; the
// palette, the commands that apply when it opens.
import { useMemo } from "react";

import { useCommands } from "../commands/registry";
import { useFiles } from "../data/files";
import { closeMenu, closePrompt, dismissNotice, ranCommand, useOverlays } from "../data/overlays";
import { useClient } from "../data/session";
import { Menu } from "../features/menu/Menu";
import { Notices } from "../features/notice/Notices";
import { CommandPalette } from "../features/prompt/CommandPalette";
import { QuickSwitcher, type SwitcherWhere } from "../features/prompt/QuickSwitcher";
import { formatHotkey, type HotkeyCommand, hotkeysFor } from "../lib/keymap";
import type { PaletteCommand } from "../lib/palette";
import type { SwitcherNote } from "../lib/switcher";
import { type OpenWhere, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { openOrCreate, viewOf } from "./commands";

const WHERE: Record<SwitcherWhere, OpenWhere> = { current: "here", tab: "tab", right: "right" };

/** Overlays draws the window's menu, prompt and notices. */
export function Overlays() {
  const menu = useOverlays((s) => s.menu);
  const prompt = useOverlays((s) => s.prompt);
  const notices = useOverlays((s) => s.notices);
  return (
    <>
      {menu && (
        <Menu
          label={menu.label}
          items={menu.items}
          x={menu.x}
          y={menu.y}
          onChoose={menu.onChoose}
          onClose={closeMenu}
        />
      )}
      {prompt === "switcher" && <Switcher />}
      {prompt === "palette" && <Palette />}
      <Notices notices={notices} onDismiss={dismissNotice} />
    </>
  );
}

function Switcher() {
  const client = useClient();
  const files = useFiles((s) => s.files);
  const recent = useWorkspace((s) => s.ws.recent);
  const update = useWorkspace((s) => s.update);
  // Aliases join when note.list carries them; until then a note matches by its path.
  const notes = useMemo<SwitcherNote[]>(
    () =>
      [...files.values()]
        .filter((f) => f.kind === "note")
        .map((f) => ({ path: f.path, aliases: [] }))
        .sort((a, b) => a.path.localeCompare(b.path)),
    [files],
  );
  return (
    <QuickSwitcher
      notes={notes}
      recent={recent}
      onOpen={(path, where) => update((w) => open(w, viewOf(path), WHERE[where]))}
      onCreate={(name, where) => void openOrCreate(client, name, WHERE[where])}
      onClose={closePrompt}
    />
  );
}

function Palette() {
  const registry = useCommands();
  const recent = useOverlays((s) => s.recentCommands);
  // The commands that apply when the palette opens.
  const commands = useMemo<PaletteCommand[]>(
    () =>
      registry.list().map((c) => ({
        id: c.id,
        name: c.name,
        keys: hotkeysFor(c.id as HotkeyCommand).map(formatHotkey),
      })),
    [registry],
  );
  return (
    <CommandPalette
      commands={commands}
      recent={recent}
      onRun={(id) => {
        ranCommand(id);
        registry.run(id);
      }}
      onClose={closePrompt}
    />
  );
}
