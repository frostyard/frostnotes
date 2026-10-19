// Validate a new vault's name and join it to its chosen parent folder
// (docs/specs/ui.md#vault-manager).

/** VAULT_NAME_FORBIDDEN are the characters a vault's name may not hold: those a note's title may not. */
export const VAULT_NAME_FORBIDDEN = /[\\/:*?"<>|#^[\]]/;

/** vaultNameError says why name cannot name a new vault, or null when it can. */
export function vaultNameError(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) return "A vault needs a name.";
  if (trimmed.startsWith(".")) return "A vault's name cannot start with a dot.";
  if (VAULT_NAME_FORBIDDEN.test(trimmed)) {
    return "A vault's name cannot hold any of these: \\ / : * ? \" < > | # ^ [ ]";
  }
  return null;
}

/** vaultFolder is the path of the vault named name in the folder location. */
export function vaultFolder(location: string, name: string): string {
  return `${location.replace(/\/+$/, "")}/${name.trim()}`;
}
