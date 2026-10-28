// In a browser, and so in lodge's web build, notesd is a service of the
// machine: there is no background setting to read or to store.
import { expect, it } from "vitest";

import { readBackground, setBackground } from "./background";

it("reads and stores nothing outside the desktop app", async () => {
  await expect(readBackground()).resolves.toBeNull();
  await expect(setBackground(false)).resolves.toBeNull();
});
