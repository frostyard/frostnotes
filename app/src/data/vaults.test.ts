// notesd's refusals of vault.add, in the person's words (ADR-0024;
// docs/specs/ui.md#vault-manager). The messages are notesd's
// (internal/engine/vault.go) and the mock's.
import { describe, expect, it } from "vitest";

import { ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { refusal } from "./vaults";

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
