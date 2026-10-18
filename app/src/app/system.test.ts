// The webview's own context menu never shows (review finding 5, plan 0005
// Phase 7; docs/specs/ui.md, Menus).
import { describe, expect, it } from "vitest";

import { suppressNativeMenu } from "./system";

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
