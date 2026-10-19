import { describe, expect, it } from "vitest";

import { base64url, contentMAC, fromBase64url, hmacSha256 } from "./sha256";

const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
const enc = (s: string) => new TextEncoder().encode(s);

describe("HMAC-SHA256", () => {
  it("matches RFC 4231's test cases", () => {
    expect(hex(hmacSha256(enc("Jefe"), enc("what do ya want for nothing?")))).toBe(
      "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
    );
    // Test case 6: a key longer than the block is hashed first.
    expect(
      hex(hmacSha256(new Uint8Array(131).fill(0xaa), enc("Test Using Larger Than Block-Size Key - Hash Key First"))),
    ).toBe("60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54");
  });
});

describe("content MACs", () => {
  it("name a file as notesd's blobhttp.ContentMAC does", () => {
    const key = fromBase64url("BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc");
    expect(key).toEqual(new Uint8Array(32).fill(7));
    expect(contentMAC(key, "0".repeat(64))).toBe("VPhZRCaVPlqKuNif504xfJGTfzO65htaLMlSsnyRli8");
    expect(contentMAC(key, "2e39f08959218012438d1b58a05b73fe326f02823732d5201cb330225869bfe0")).toBe(
      "_EaRzyTYmC7x07Ow7pkgAOuDu2UJwrSJRXnq3AJoRfg",
    );
    expect(base64url(key)).toBe("BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc");
  });
});
