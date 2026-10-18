// What the window draws over its layout (data/overlays.ts): the open
// menu, the quick switcher or the command palette, and the notices. The
// switcher lists the vault's notes and the workspace's recent files; the
// palette, the commands that apply when it opens.
import { useMemo } from "react";

import { useCommands } from "../commands/registry";
import { useAliases } from "../data/aliases";
import { useFiles } from "../data/files";
import {
  closeMenu,
  closeMove,
  closePreview,
  closePrompt,
  closeTemplates,
  dismissNotice,
  notify,
  ranCommand,
  useOverlays,
} from "../data/overlays";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { titleOf } from "../editor/NoteEditor";
import { useDocuments } from "../editor/registry";
import { ConfirmDialog } from "../features/dialog/ConfirmDialog";
import { Menu } from "../features/menu/Menu";
import { Notices } from "../features/notice/Notices";
import { PagePreview } from "../features/preview/PagePreview";
import { CommandPalette } from "../features/prompt/CommandPalette";
import { FolderChooser } from "../features/prompt/FolderChooser";
import { QuickSwitcher, type SwitcherWhere } from "../features/prompt/QuickSwitcher";
import { TemplateChooser } from "../features/prompt/TemplateChooser";
import { formatHotkey, type HotkeyCommand, hotkeysFor } from "../lib/keymap";
import type { PaletteCommand } from "../lib/palette";
import type { SwitcherNote } from "../lib/switcher";
import { type OpenWhere, open } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { insertTemplate, openOrCreate, viewOf } from "./commands";
import { move } from "./panes/files";
import { SettingsContainer } from "./Settings";
import { VaultsView } from "./VaultsView";

const WHERE: Record<SwitcherWhere, OpenWhere> = { current: "here", tab: "tab", right: "right" };

/** Overlays draws the window's menu, prompt and notices. */
export function Overlays() {
  const menu = useOverlays((s) => s.menu);
  const prompt = useOverlays((s) => s.prompt);
  const notices = useOverlays((s) => s.notices);
  const confirm = useOverlays((s) => s.confirm);
  const moving = useOverlays((s) => s.move);
  const templates = useOverlays((s) => s.templates);
  const preview = useOverlays((s) => s.preview);
  const vaults = useOverlays((s) => s.vaults);
  return (
    <>
      <SettingsContainer />
      {vaults && <VaultsView mode="dialog" />}
      {moving && <Mover path={moving.path} folder={moving.folder} />}
      {templates && <Templates paths={templates} />}
      {preview && (
        <PagePreview
          anchor={preview.anchor}
          title={titleOf(preview.path)}
          fill={preview.fill}
          onClose={closePreview}
          onOpen={() => {
            closePreview();
            useWorkspace.getState().update((w) => open(w, viewOf(preview.path), "here"));
          }}
        />
      )}
      {confirm && (
        <ConfirmDialog
          title={confirm.title}
          text={confirm.text}
          confirmLabel={confirm.confirmLabel}
          cancelLabel={confirm.cancelLabel}
          danger={confirm.danger}
          onConfirm={() => confirm.answer(true)}
          onCancel={() => confirm.answer(false)}
        />
      )}
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
  const aliases = useAliases((s) => s.byPath);
  const notes = useMemo<SwitcherNote[]>(
    () =>
      [...files.values()]
        .filter((f) => f.kind === "note")
        .map((f) => ({ path: f.path, aliases: aliases.get(f.path) ?? [] }))
        .sort((a, b) => a.path.localeCompare(b.path)),
    [files, aliases],
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

// Templates is the template chooser inserting the template chosen into the active editor.
function Templates({ paths }: { paths: readonly string[] }) {
  const client = useClient();
  const docs = useDocuments();
  return (
    <TemplateChooser
      templates={paths}
      onClose={closeTemplates}
      onChoose={(path) => void insertTemplate(client, docs, path)}
    />
  );
}

// Mover is the folder chooser moving a file or folder into the folder chosen.
function Mover({ path, folder }: { path: string; folder: boolean }) {
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const folders = useFiles((s) => s.folders);
  const name = path.slice(path.lastIndexOf("/") + 1);
  // A folder cannot go into itself or below it.
  const choices = useMemo(
    () => (folder ? folders.filter((f) => f !== path && !f.startsWith(`${path}/`)) : folders),
    [folders, folder, path],
  );
  return (
    <FolderChooser
      folders={choices}
      placeholder={`Move "${name.replace(/\.md$/, "")}" to…`}
      onClose={closeMove}
      onChoose={(to) => {
        if (vaultId === null) return;
        move(client, vaultId, path, to === "" ? name : `${to}/${name}`, folder).catch((err: unknown) =>
          notify(err instanceof Error ? err.message : String(err)),
        );
      }}
    />
  );
}
