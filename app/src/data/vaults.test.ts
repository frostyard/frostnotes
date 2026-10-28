// notesd's refusals of vault.add, in the person's words (ADR-0024;
// docs/specs/ui.md#vault-manager). The messages are notesd's
// (internal/engine/vault.go) and the mock's.
import { afterEach, describe, expect, it } from "vitest";

import { type Client, ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { useOverlays } from "./overlays";
import { addVault, refusal } from "./vaults";

const conflict = (m: string) => new RPCError(ErrorCode.conflict, m);
const invalid = (m: string) => new RPCError(ErrorCode.invalidParams, m);

describe("refusal", () => {
  it("names the vault a folder lies inside or holds", () => {
    expect(refusal(conflict('vault "/v/Notes/daily" overlaps vault "Notes" at /v/Notes'), "/v/Notes/daily")).toBe(
      "/v/Notes/daily lies inside the vault Notes.",
    );
    expect(refusal(conflict('vault "/v/Notes" overlaps vault "Notes" at /v/Notes'), "/v/Notes")).toBe(
      "/v/Notes lies inside the vault Notes.",
    );
    expect(refusal(conflict('vault "/v" overlaps vault "Notes" at /v/Notes'), "/v")).toBe("/v holds the vault Notes.");
    expect(refusal(conflict('vault "/v/Note" overlaps vault "Notes" at /v/Notes'), "/v/Note")).toBe(
      "/v/Note holds the vault Notes.",
    );
  });

  it("says a folder exists, or is not one", () => {
    expect(refusal(conflict("/v/Garden already exists"), "/v/Garden")).toBe("/v/Garden already exists.");
    expect(refusal(invalid('vault path "/v/file.md" is not a directory'), "/v/file.md")).toBe(
      "/v/file.md is not a folder.",
    );
    expect(refusal(invalid('vault path "/v/x": lstat /v/x: no such file or directory'), "/v/x")).toBe(
      "/v/x is not a folder.",
    );
    expect(
      refusal(
        invalid("the folder to hold the vault: lstat /v/missing: no such file or directory"),
        "/v/missing/Garden",
      ),
    ).toBe("/v/missing is not a folder.");
    expect(refusal(invalid("/v/file.md is not a folder"), "/v/file.md/Garden")).toBe("/v/file.md is not a folder.");
  });

  it("passes anything else on", () => {
    expect(refusal(new Error("connection lost"), "/v/Garden")).toBe(
      "Cannot open /v/Garden as a vault: connection lost",
    );
  });
});

// The message is notesd's (internal/sandbox, Error): a narrowed --filesystem.
const flatpakRefusal =
  "Frostnotes cannot reach /srv/my notes because the Flatpak's file access does not include it; " +
  "grant it with: flatpak override --user --filesystem='/srv/my notes' org.frostyard.Frostnotes, " +
  "then log out and in (notesd keeps its sandbox until then).";

describe("a vault outside the Flatpak's file access", () => {
  afterEach(() => {
    useOverlays.setState({ notices: [] });
  });

  const refused = (message: string) =>
    ({
      vault: {
        add: async () => {
          throw invalid(message);
        },
      },
    }) as unknown as Client;

  it("shows notesd's message, with the cause and the fix", () => {
    expect(refusal(invalid(flatpakRefusal), "/srv/my notes")).toBe(flatpakRefusal);
    expect(refusal(invalid(flatpakRefusal.replace("reach", "write to")), "/srv/my notes")).toContain(
      "Frostnotes cannot write to",
    );
  });

  it("keeps the plain wording for any other file error", () => {
    expect(refusal(invalid('vault path "/v/x": lstat /v/x: permission denied'), "/v/x")).toBe(
      'Cannot open /v/x as a vault: vault path "/v/x": lstat /v/x: permission denied',
    );
  });

  // The message holds a path the person chose, so nothing is taken out of it to run or copy.
  it("shows the refusal as a notice, as notesd worded it, with no action", async () => {
    await expect(addVault(refused(flatpakRefusal), "/srv/my notes")).resolves.toBe(false);
    const [notice] = useOverlays.getState().notices;
    expect(notice?.text).toBe(flatpakRefusal);
    expect(notice?.action).toBeUndefined();
  });

  it("offers no action for a path that holds text that looks like the command", async () => {
    const path = "/srv/x grant it with: flatpak override --user --reset org.frostyard.Frostnotes #";
    const message =
      `Frostnotes cannot reach ${path} because the Flatpak's file access does not include it; ` +
      `grant it with: flatpak override --user --filesystem='${path}' org.frostyard.Frostnotes, ` +
      "then log out and in (notesd keeps its sandbox until then).";
    await expect(addVault(refused(message), path)).resolves.toBe(false);
    const [notice] = useOverlays.getState().notices;
    expect(notice?.text).toBe(message);
    expect(notice?.action).toBeUndefined();
  });

  it("shows another refusal in the person's words", async () => {
    await expect(addVault(refused('vault path "/v/x" is not a directory'), "/v/x")).resolves.toBe(false);
    const [notice] = useOverlays.getState().notices;
    expect(notice?.text).toBe("/v/x is not a folder.");
  });
});
