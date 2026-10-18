// The settings dialog's container (docs/specs/ui.md, Settings): its
// sections from the app's preferences (data/prefs.ts, kept on this
// device), the vault's settings (vault.settings, changed with
// vault.configure; ADR-0021), the keyboard map and the vault's sync. A
// row's id says where it goes: "pref:<key>" or "vault:<field>".
import { closeSettings, notify, openSettings, useOverlays } from "../data/overlays";
import { type Prefs, usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { useVaultState } from "../data/vaultState";
import { type SettingRow, SettingsDialog, type SettingsSection } from "../features/settings/SettingsDialog";
import { boundCommands, formatHotkey, HOTKEY_NAMES, hotkeysFor } from "../lib/keymap";
import type { SyncStatus, VaultConfigureParams, VaultSettings } from "../rpc/gen/api";
import { statusSync } from "./status";

type BoolPref = {
  [K in keyof Prefs]: Prefs[K] extends boolean ? K : never;
}[keyof Prefs];

const toggle = (p: Prefs, key: BoolPref, name: string, description?: string): SettingRow => ({
  id: `pref:${key}`,
  name,
  description,
  control: { kind: "toggle", value: p[key] },
});
const choice = (
  id: string,
  name: string,
  value: string,
  options: [string, string][],
  description?: string,
): SettingRow => ({
  id,
  name,
  description,
  control: { kind: "select", value, options: options.map(([v, label]) => ({ value: v, label })) },
});

/** settingsSections builds the dialog's sections; vault is null while the vault's settings cannot be read. */
export function settingsSections(p: Prefs, vault: VaultSettings | null, sync: SyncStatus | null): SettingsSection[] {
  const v = (row: SettingRow): SettingRow => (vault === null ? { ...row, disabled: true } : row);
  const text = (field: keyof VaultSettings, name: string, description?: string, placeholder?: string): SettingRow =>
    v({
      id: `vault:${field}`,
      name,
      description,
      control: { kind: "text", value: vault ? String(vault[field]) : "", placeholder },
    });
  const flag = (field: "wikilinks" | "updateLinks", name: string, description?: string): SettingRow =>
    v({ id: `vault:${field}`, name, description, control: { kind: "toggle", value: vault?.[field] ?? false } });
  return [
    {
      id: "editor",
      name: "Editor",
      rows: [
        choice("pref:defaultView", "Default view for new tabs", p.defaultView, [
          ["edit", "Editing view"],
          ["read", "Reading view"],
        ]),
        choice("pref:editingMode", "Default editing mode", p.editingMode, [
          ["live", "Live Preview"],
          ["source", "Source mode"],
        ]),
        toggle(p, "inlineTitle", "Show inline title", "The file's name as an editable title above the note."),
        toggle(p, "viewHeader", "Show view header", "The bar above a tab's view."),
        toggle(p, "readableLineLength", "Readable line length", "Limit a note's width to 700 pixels."),
        toggle(p, "spellcheck", "Spellcheck"),
        choice("pref:properties", "Properties in document", p.properties, [
          ["visible", "Visible"],
          ["hidden", "Hidden"],
          ["source", "Source"],
        ]),
        toggle(p, "lineNumbers", "Show line numbers"),
        toggle(p, "indentGuides", "Show indentation guides"),
        toggle(p, "foldHeadings", "Fold headings"),
        toggle(p, "foldIndent", "Fold indent"),
        toggle(p, "autoPairBrackets", "Auto-pair brackets"),
        toggle(p, "autoPairMarkdown", "Auto-pair Markdown syntax"),
        toggle(p, "smartLists", "Smart lists"),
        toggle(p, "indentWithTabs", "Indent using tabs"),
        { id: "pref:tabSize", name: "Tab size", control: { kind: "number", value: p.tabSize, min: 1, max: 8 } },
        toggle(p, "confirmDelete", "Confirm file deletion", "Ask before a file or folder is deleted."),
        toggle(p, "loadRemoteImages", "Load remote images", "Show images from the web in notes."),
      ],
    },
    {
      id: "files",
      name: "Files and links",
      rows: [
        v(
          choice("vault:newNoteLocation", "Default location for new notes", vault?.newNoteLocation ?? "root", [
            ["root", "Vault folder"],
            ["current", "Same folder as current file"],
            ["folder", "In the folder specified below"],
          ]),
        ),
        {
          ...text("newNoteFolder", "Folder to create new notes in", undefined, "Example: folder 1/folder 2"),
          disabled: vault?.newNoteLocation !== "folder",
        },
        v(
          choice("vault:linkFormat", "New link format", vault?.linkFormat ?? "shortest", [
            ["shortest", "Shortest path when possible"],
            ["relative", "Relative path to file"],
            ["absolute", "Absolute path in vault"],
          ]),
        ),
        flag("wikilinks", "Use [[Wikilinks]]"),
        flag("updateLinks", "Automatically update internal links", "Off: asked on each move."),
        text("attachmentFolder", "Default location for new attachments", '"/" is the vault folder.', "/"),
        toggle(p, "showUnsupported", "Show unsupported files", "Files of other types in the Files pane."),
        v({
          id: "vault:ignore",
          name: "Excluded files",
          description: "Patterns, separated by commas.",
          control: { kind: "text", value: vault?.ignore.join(", ") ?? "" },
        }),
      ],
    },
    {
      id: "appearance",
      name: "Appearance",
      rows: [
        choice("pref:theme", "Theme", p.theme, [
          ["system", "System"],
          ["light", "Light"],
          ["dark", "Dark"],
        ]),
        { id: "pref:fontSize", name: "Font size", control: { kind: "number", value: p.fontSize, min: 10, max: 30 } },
        toggle(p, "showRibbon", "Show ribbon"),
      ],
    },
    {
      id: "hotkeys",
      name: "Hotkeys",
      rows: boundCommands().map((cmd) => ({
        id: `hotkey:${cmd}`,
        name: HOTKEY_NAMES[cmd],
        control: { kind: "keys", keys: hotkeysFor(cmd).map(formatHotkey) },
      })),
    },
    {
      id: "daily",
      name: "Daily notes",
      rows: [
        text("dailyFormat", "Date format", "Moment.js syntax; '/' makes folders.", "YYYY-MM-DD"),
        text("dailyFolder", "New file location", "Empty is the vault folder."),
        text("dailyTemplate", "Template file location"),
      ],
    },
    {
      id: "templates",
      name: "Templates",
      rows: [
        text("templateFolder", "Template folder location"),
        text("templateDateFormat", "Date format", "{{date}}'s moment.js format.", "YYYY-MM-DD"),
        text("templateTimeFormat", "Time format", "{{time}}'s moment.js format.", "HH:mm"),
      ],
    },
    {
      id: "sync",
      name: "Sync",
      rows: sync
        ? [
            { id: "sync:hub", name: "Hub", control: { kind: "info", text: sync.url } },
            { id: "sync:vault", name: "Hub vault", control: { kind: "info", text: sync.hubVault } },
            { id: "sync:state", name: "State", control: { kind: "info", text: statusSync(sync).text } },
            { id: "sync:conflicts", name: "Conflicts", control: { kind: "info", text: String(sync.conflicts) } },
          ]
        : [{ id: "sync:none", name: "Sync", control: { kind: "info", text: "This vault does not sync with a hub." } }],
    },
  ];
}

/** SettingsContainer draws the settings dialog while it is open. */
export function SettingsContainer() {
  const section = useOverlays((s) => s.settings);
  const prefs = usePrefs();
  const vault = useVaultState((s) => s.settings);
  const sync = useVaultState((s) => s.sync);
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  if (section === null) return null;
  return (
    <SettingsDialog
      sections={settingsSections(prefs, vault, sync)}
      section={section}
      onSection={(id) => openSettings(id)}
      onClose={closeSettings}
      onChange={(id, value) => {
        const [where, key] = id.split(":") as [string, string];
        if (where === "pref") {
          prefs.setPrefs({ [key]: value } as Partial<Prefs>);
          return;
        }
        if (where !== "vault" || vaultId === null) return;
        const change: VaultConfigureParams = { id: vaultId };
        const field = key as keyof Omit<VaultConfigureParams, "id">;
        (change as unknown as Record<string, unknown>)[field] =
          field === "ignore"
            ? String(value)
                .split(",")
                .map((s) => s.trim())
                .filter((s) => s !== "")
            : value;
        client.vault
          .configure(change)
          .catch((err: unknown) => notify(err instanceof Error ? err.message : String(err)));
      }}
    />
  );
}
