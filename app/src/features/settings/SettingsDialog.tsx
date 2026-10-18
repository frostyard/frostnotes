// The settings dialog (docs/specs/ui.md, Settings). Task T-0060 replaces
// this stub.

/** SettingControl is a setting's control and value. */
export type SettingControl =
  | { kind: "toggle"; value: boolean }
  | { kind: "select"; value: string; options: readonly { value: string; label: string }[] }
  | { kind: "text"; value: string; placeholder?: string }
  | { kind: "number"; value: number; min: number; max: number }
  /** Read-only key caps (the Hotkeys section). */
  | { kind: "keys"; keys: readonly string[] }
  /** Read-only text. */
  | { kind: "info"; text: string };

/** SettingRow is one setting. */
export interface SettingRow {
  id: string;
  name: string;
  description?: string;
  control: SettingControl;
  disabled?: boolean;
}

/** SettingsSection is one section of the dialog. */
export interface SettingsSection {
  id: string;
  name: string;
  rows: readonly SettingRow[];
}

export interface SettingsDialogProps {
  sections: readonly SettingsSection[];
  /** The shown section's id. */
  section: string;
  onSection: (id: string) => void;
  /** A setting changed: a toggle's boolean, a select's or text's string, a number's number. */
  onChange: (id: string, value: boolean | string | number) => void;
  onClose: () => void;
}

/** SettingsDialog draws the settings dialog in a portal on the document's body. */
export function SettingsDialog(_props: SettingsDialogProps) {
  return null;
}
