// The webview's own context menu never shows (review finding 5, plan 0005
// Phase 7; docs/specs/ui.md, Menus).
import { describe, expect, it } from "vitest";

import { canPickFolders, pickFolder, setWindowTitle, suppressNativeMenu } from "./system";

describe("suppressNativeMenu", () => {
  it("keeps the webview's menu from showing anywhere, until removed", () => {
    const remove = suppressNativeMenu();
    const p = document.createElement("p");
    document.body.append(p);
    const ask = () => {
      const e = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
      p.dispatchEvent(e);
      return e.defaultPrevented;
    };
    expect(ask()).toBe(true);
    remove();
    expect(ask()).toBe(false);
    p.remove();
  });
});

describe("the folder picker and the window's title in a browser", () => {
  it("has no picker, and names the page", async () => {
    expect(canPickFolders()).toBe(false);
    await expect(pickFolder("Open folder as vault")).resolves.toBeNull();
    setWindowTitle("Plan - Notes - Frostnotes");
    expect(document.title).toBe("Plan - Notes - Frostnotes");
  });
});
