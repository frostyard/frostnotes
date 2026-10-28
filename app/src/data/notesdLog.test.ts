// In a browser, and so in lodge's web build, there is no notesd.log to read.
import { expect, it } from "vitest";

import { readNotesdLog } from "./notesdLog";

it("reads nothing outside the desktop app", async () => {
  await expect(readNotesdLog()).resolves.toBeNull();
});
