// The app's sync flow without a terminal (docs/design/sync.md#pairing-and-sessions):
// the steps `notesctl hub join` runs (cmd/notesctl/hub.go) — pair with the
// hub unless this device paired with it before, show the code and wait for
// its approval, then join — plus pairing again and leaving a hub
// (docs/specs/ui.md#vault-manager, docs/specs/ui.md#settings).
import { type Client, ErrorCode, type Pairing, type SyncStatus } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

/** JoinTarget is a local vault and the hub vault to sync it with. */
export interface JoinTarget {
  vaultId: number;
  /** The hub's URL. */
  url: string;
  /** The hub vault's name. */
  name: string;
}

/** JoinRequest is JoinTarget with this device's name on the hub. */
export interface JoinRequest extends JoinTarget {
  /** This device's name on the hub; notesd uses the host name without one. */
  device?: string;
}

/**
 * Cancelled says the person stopped the flow between two calls. notesd
 * long-polls one call at a time, so a cancelled flow leaves nothing running.
 */
export class Cancelled extends Error {
  constructor() {
    super("The join was cancelled.");
    this.name = "AbortError";
  }
}

/** isCancelled reports whether the person stopped the flow, rather than it failing. */
export function isCancelled(err: unknown): boolean {
  return err instanceof Cancelled || (err instanceof Error && err.name === "AbortError");
}

/** hubURL is a hub's URL as this flow uses it: trimmed, with no trailing "/". */
export function hubURL(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/** syncError says an error in the person's words: notesd's own message. */
export function syncError(err: unknown): string {
  return err instanceof Error && err.message !== "" ? err.message : String(err);
}

/** stop ends the flow once the person cancelled it. */
function stop(signal: AbortSignal | undefined): void {
  if (signal?.aborted) throw new Cancelled();
}

/** hubPaired reports whether this device paired with the hub at url before. */
async function hubPaired(c: Client, url: string): Promise<boolean> {
  const hubs = await c.sync.hubs();
  return hubs.some((h) => h.url === url);
}

/**
 * awaitApproval shows the pairing's code and waits until the hub's person
 * approves it. notesd long-polls up to 30 seconds for each sync.paired, so
 * a pending pairing asks again without waiting here.
 */
async function awaitApproval(
  c: Client,
  pairing: Pairing,
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<void> {
  stop(signal);
  onCode(pairing.code);
  for (;;) {
    stop(signal);
    const result = await c.sync.paired({ pairing: pairing.pairing });
    switch (result.state) {
      case "approved":
        return;
      case "expired":
        throw new Error("The pairing expired: the code was not approved within 10 minutes.");
      case "pending":
        break;
    }
  }
}

/**
 * pairNew pairs with the hub at url and waits for the approval of its code.
 * A pairing with a hub this device paired with before replaces its token.
 */
async function pairNew(
  c: Client,
  url: string,
  device: string | undefined,
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<void> {
  stop(signal);
  const pairing = await c.sync.pair(device === undefined ? { url } : { url, name: device });
  await awaitApproval(c, pairing, onCode, signal);
}

/**
 * ensurePaired pairs with the hub at url unless this device paired with it
 * before, showing the code to approve. It reports whether the device was
 * paired before: a hub that refuses the token of such a device needs a new
 * pairing, and a join then runs once more (joinPaired).
 */
export async function ensurePaired(
  c: Client,
  url: string,
  device: string | undefined,
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<boolean> {
  const hub = hubURL(url);
  stop(signal);
  const paired = await hubPaired(c, hub);
  stop(signal);
  if (!paired) await pairNew(c, hub, device, onCode, signal);
  return paired;
}

/**
 * joinPaired joins the vault with the hub, after ensurePaired. Where the hub
 * refuses the token of a device that was paired (`revoked`: the hub revoked
 * the device, or the device lost the token), it pairs again once and joins
 * again (frostnotes PR #101; cmd/notesctl/hub.go).
 */
export async function joinPaired(
  c: Client,
  target: JoinTarget,
  paired: boolean,
  device: string | undefined,
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<SyncStatus> {
  const join = { vaultId: target.vaultId, url: hubURL(target.url), name: target.name };
  stop(signal);
  try {
    return await c.sync.join(join);
  } catch (err) {
    if (!paired || !(err instanceof RPCError) || err.code !== ErrorCode.revoked) throw err;
    stop(signal);
    await pairNew(c, join.url, device, onCode, signal);
    stop(signal);
    return await c.sync.join(join);
  }
}

/** joinHub runs `notesctl hub join`'s steps for a vault that exists already: pair where needed, then join. */
export async function joinHub(
  c: Client,
  p: JoinRequest,
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<SyncStatus> {
  const paired = await ensurePaired(c, p.url, p.device, onCode, signal);
  return joinPaired(c, p, paired, p.device, onCode, signal);
}

/**
 * pairAgain pairs again with the hub at url, replacing the hub's token: the
 * step that syncs a vault in state revoked, and every vault on that hub,
 * again with no join.
 */
export async function pairAgain(
  c: Client,
  p: { url: string; device?: string },
  onCode: (code: string) => void,
  signal?: AbortSignal,
): Promise<void> {
  await pairNew(c, hubURL(p.url), p.device, onCode, signal);
}

/** leaveHub stops syncing a vault; its files stay on this device, and the hub keeps its copy. */
export async function leaveHub(c: Client, vaultId: number): Promise<void> {
  await c.sync.leave({ vaultId });
}
