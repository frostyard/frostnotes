// The settings dialog's container (docs/specs/ui.md, Settings): its
// sections from the app's preferences (data/prefs.ts, kept on this
// device), the vault's settings (vault.settings, changed with
// vault.configure; ADR-0021), the keyboard map, the vault's sync, and on
// a hub its devices and pending pairings (docs/design/hub.md#hub-settings-in-the-app).
// A row's id says where it goes: "pref:<key>", "vault:<field>" or
// "hub:revoke:<device>".
import { useEffect, useState } from "react";

import { ask, closeSettings, notify, openSettings, useOverlays } from "../data/overlays";
import { type Prefs, usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { useVaultState } from "../data/vaultState";
import { type SettingRow, SettingsDialog, type SettingsSection } from "../features/settings/SettingsDialog";
import { boundCommands, formatHotkey, HOTKEY_NAMES, hotkeysFor } from "../lib/keymap";
import type {
  Client,
  HubDevice,
  HubStatus,
  PendingPairing,
  SyncStatus,
  VaultConfigureParams,
  VaultSettings,
} from "../rpc/gen/api";
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

/** HubView is this notesd's hub role, for the Hub section. */
export interface HubView {
  status: HubStatus;
  devices: HubDevice[];
  pending: PendingPairing[];
}

/** loadHub reads the hub's state; null where the caller may not, or notesd is no hub. */
export async function loadHub(c: Client): Promise<HubView | null> {
  try {
    const [status, devices, pending] = await Promise.all([c.hub.status(), c.hub.devices(), c.hub.pending()]);
    return status.hub === "" ? null : { status, devices, pending };
  } catch {
    return null;
  }
}

const time = (at: string) => at.replace("T", " ").replace(/:\d\d(\.\d+)?Z$/, " UTC");

/**
 * hubSection lists the hub's devices, each with Revoke while it is not
 * revoked, and its pending pairings, each with the node lodge named and
 * the command that approves it on the hub's machine: approving is local
 * only (ADR-0016).
 */
export function hubSection(hub: HubView): SettingsSection {
  const rows: SettingRow[] = [{ id: "hub:id", name: "Hub", control: { kind: "info", text: hub.status.hub } }];
  if (hub.status.url)
    rows.push({ id: "hub:url", name: "Devices' URL", control: { kind: "info", text: hub.status.url } });
  for (const d of hub.devices) {
    const seen = d.seenAt ? `, last seen ${time(d.seenAt)}` : ", never seen";
    rows.push({
      id: `hub:revoke:${d.id}`,
      name: d.name,
      description: `Paired ${time(d.pairedAt)}${seen}${d.connected ? ", connected" : ""}`,
      control: d.revoked ? { kind: "info", text: "Revoked" } : { kind: "button", label: "Revoke", danger: true },
    });
  }
  if (hub.devices.length === 0) {
    rows.push({ id: "hub:none", name: "Devices", control: { kind: "info", text: "No device has paired." } });
  }
  for (const pp of hub.pending) {
    rows.push({
      id: `hub:pending:${pp.code}`,
      name: `${pp.name} (pending)`,
      description: `From ${pp.addr || "a node lodge did not name"}; expires ${time(pp.expiresAt)}`,
      control: { kind: "info", text: pp.approved ? "Approved" : `notesctl hub approve ${pp.code}` },
    });
  }
  return { id: "hub", name: "Hub", rows };
}

/** settingsSections builds the dialog's sections; vault is null while the vault's settings cannot be read. */
export function settingsSections(
  p: Prefs,
  vault: VaultSettings | null,
  sync: SyncStatus | null,
  hub: HubView | null = null,
): SettingsSection[] {
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
    ...(hub ? [hubSection(hub)] : []),
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
  const [hub, setHub] = useState<HubView | null>(null);
  const open = section !== null;
  useEffect(() => {
    if (!open) return;
    let live = true;
    void loadHub(client).then((h) => live && setHub(h));
    return () => {
      live = false;
    };
  }, [open, client]);
  if (section === null) return null;
  return (
    <SettingsDialog
      sections={settingsSections(prefs, vault, sync, hub)}
      section={section}
      onSection={(id) => openSettings(id)}
      onClose={closeSettings}
      onChange={(id, value) => {
        const [where, key, arg] = id.split(":") as [string, string, string | undefined];
        if (where === "hub" && key === "revoke" && arg !== undefined) {
          const device = hub?.devices.find((d) => d.id === Number(arg));
          void ask({
            title: `Revoke ${device?.name ?? "this device"}?`,
            text: "Its token stops working at once, and it must pair again to sync.",
            confirmLabel: "Revoke",
            danger: true,
          }).then(async (yes) => {
            if (!yes) return;
            try {
              await client.hub.revoke({ device: Number(arg) });
            } catch (err) {
              notify(err instanceof Error ? err.message : String(err));
            }
            setHub(await loadHub(client));
          });
          return;
        }
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
