// Vaults from the app (ADR-0024; docs/specs/ui.md#vault-manager): Open
// folder as vault and Create new vault register a folder with notesd and
// make it the window's vault; Remove from list forgets one after asking.
// notesd's refusals show as notices in the person's words.
import { type Client, ErrorCode, type Vault } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { ask, notify } from "./overlays";
import { openVault } from "./session";
import { byName, useNotes } from "./stores";

/** GRANT finds the command that grants the Flatpak a folder in notesd's message (internal/sandbox). */
const GRANT = /^Frostnotes cannot .* grant it with: (flatpak override .+), then log out and in/;

/**
 * addVault registers the folder at path, made first when create is set,
 * and makes it the window's vault; a refusal shows as a notice. It
 * reports whether the vault was added.
 */
export async function addVault(c: Client, path: string, create = false): Promise<boolean> {
  try {
    const vault = await c.vault.add(create ? { path, create: true } : { path });
    // vault.added follows as an event; the list has the vault before the window opens it.
    useNotes.setState((s) => ({
      vaults: s.vaults.some((v) => v.id === vault.id) ? s.vaults : [...s.vaults, vault].sort(byName),
    }));
    await openVault(c, vault.id);
    return true;
  } catch (err) {
    const text = refusal(err, path);
    // The Flatpak's file access is the cause: the notice carries the fix to paste.
    const fix = GRANT.exec(text)?.[1];
    notify(text, fix === undefined ? undefined : { label: "Copy command", run: () => void copy(fix) });
    return false;
  }
}

async function copy(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    notify("Copied the command.");
  } catch {
    notify("Cannot copy to the clipboard.");
  }
}

/** refusal says in the person's words why notesd would not add the vault at path. */
export function refusal(err: unknown, path: string): string {
  const text = err instanceof Error ? err.message : String(err);
  if (err instanceof RPCError && err.code === ErrorCode.conflict) {
    const overlap = /overlaps vault "(.*)" at (.*)$/.exec(text);
    if (overlap) {
      const [, name, at] = overlap;
      const inside = at !== undefined && (path === at || path.startsWith(`${at.replace(/\/+$/, "")}/`));
      return inside ? `${path} lies inside the vault ${name}.` : `${path} holds the vault ${name}.`;
    }
    if (text.endsWith("already exists")) return `${path} already exists.`;
  }
  if (err instanceof RPCError && err.code === ErrorCode.invalidParams) {
    // The Flatpak's file access does not include the folder: notesd names the cause and the fix.
    if (text.startsWith("Frostnotes cannot ")) return text;
    // A new vault's parent (create), else the folder itself.
    if (text.startsWith("the folder to hold the vault") || text.endsWith("is not a folder")) {
      return `${path.slice(0, path.lastIndexOf("/")) || "/"} is not a folder.`;
    }
    if (text.includes("is not a directory") || text.includes("no such file or directory")) {
      return `${path} is not a folder.`;
    }
  }
  return `Cannot open ${path} as a vault: ${text}`;
}

/**
 * removeVault forgets a vault after asking, naming what goes: notesd's
 * index and history of it and, for a synced vault, its place on the hub;
 * never its folder or files. The window then shows another vault, or the
 * vault manager (Session follows vault.removed).
 */
export async function removeVault(c: Client, vault: Vault): Promise<void> {
  let synced = false;
  try {
    synced = (await c.sync.status({ vaultId: vault.id })).some((s) => s.vaultId === vault.id);
  } catch {
    // Not synced, or sync unavailable: nothing more to say.
  }
  const forgets = synced
    ? "Frostnotes forgets its index and version history on this device, and its place on the hub; open it again to sync it, after joining."
    : "Frostnotes forgets its index and version history on this device.";
  const yes = await ask({
    title: `Remove "${vault.name}" from the list?`,
    text: `Its folder and files stay where they are. ${forgets}`,
    confirmLabel: "Remove",
    danger: true,
  });
  if (!yes) return;
  try {
    await c.vault.remove({ id: vault.id });
  } catch (err) {
    notify(`Cannot remove ${vault.name}: ${err instanceof Error ? err.message : String(err)}`);
  }
}
