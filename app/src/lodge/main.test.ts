import { afterEach, describe, expect, it } from "vitest";

import { useNotes } from "../data/stores";
import { contentBlobs } from "./main";

afterEach(() => useNotes.setState({ hello: null }));

describe("content URLs inside lodge", () => {
  it("name a file by the MAC of its hash under the session's content key", () => {
    const blobs = contentBlobs((path) => `https://lodge-content.example.ts.net/notes/${path}`);
    expect(blobs("0".repeat(64), "png")).toBe("");
    useNotes.setState({
      hello: {
        protocol: 1,
        server: "notesd",
        caller: "session",
        contentKey: "BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc",
      },
    });
    expect(blobs("0".repeat(64), "png")).toBe(
      `https://lodge-content.example.ts.net/notes/blobs/VPhZRCaVPlqKuNif504xfJGTfzO65htaLMlSsnyRli8/${"0".repeat(64)}.png`,
    );
  });
});
