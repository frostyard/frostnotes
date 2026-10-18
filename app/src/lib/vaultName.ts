// A new vault's name and folder (docs/specs/ui.md#vault-manager). Task
// T-0083 replaces this stub.

/** VAULT_NAME_FORBIDDEN are the characters a vault's name may not hold: those a note's title may not. */
export const VAULT_NAME_FORBIDDEN = /[\\/:*?"<>|#^[\]]/;

/** vaultNameError says why name cannot name a new vault, or null when it can. */
export function vaultNameError(_name: string): string | null {
  return null;
}

/** vaultFolder is the path of the vault named name in the folder location. */
export function vaultFolder(location: string, name: string): string {
  return `${location}/${name}`;
}
