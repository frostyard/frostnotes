import { describe, expect, it } from "vitest";

import type { Vault } from "../rpc/gen/api";
import { applyVaultEvent } from "./stores";

const vault = (id: number, name: string): Vault => ({
  id,
  name,
  path: `/v/${id}`,
  createdAt: "2026-10-16T00:00:00Z",
});

describe("applyVaultEvent", () => {
  const list = [vault(2, "alpha"), vault(1, "Beta")];

  it("adds in vault.list's order and ignores a vault it already has", () => {
    const added = applyVaultEvent(list, { event: "vault.added", seq: 4, data: { vault: vault(3, "Aardvark") } });
    expect(added.map((v) => v.id)).toEqual([3, 2, 1]);
    expect(applyVaultEvent(added, { event: "vault.added", seq: 4, data: { vault: vault(3, "Aardvark") } })).toEqual(
      added,
    );
  });

  it("removes, and keeps the same list when the vault is already gone", () => {
    expect(applyVaultEvent(list, { event: "vault.removed", seq: 5, data: { id: 2 } }).map((v) => v.id)).toEqual([1]);
    expect(applyVaultEvent(list, { event: "vault.removed", seq: 6, data: { id: 9 } })).toBe(list);
  });
});
