import { afterEach, describe, expect, it } from "vitest";

import { installKeyListener } from "./keys";
import { CommandRegistry } from "./registry";

function press(target: EventTarget, key: string, mods: Partial<KeyboardEventInit> = {}): KeyboardEvent {
  const e = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...mods });
  target.dispatchEvent(e);
  return e;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("the command registry", () => {
  it("runs a command that applies, and lists those that do by name", () => {
    const r = new CommandRegistry();
    const ran: string[] = [];
    let open = false;
    const off = r.register(
      { id: "workspace:new-tab", name: "New tab", run: () => void ran.push("new") },
      {
        id: "markdown:toggle-preview",
        name: "Toggle reading view",
        applies: () => open,
        run: () => void ran.push("read"),
      },
    );
    expect(r.run("markdown:toggle-preview")).toBe(false);
    expect(r.list().map((c) => c.id)).toEqual(["workspace:new-tab"]);
    open = true;
    expect(r.run("markdown:toggle-preview")).toBe(true);
    expect(r.list().map((c) => c.name)).toEqual(["New tab", "Toggle reading view"]);
    expect(r.run("nothing:here")).toBe(false);
    off();
    expect(r.get("workspace:new-tab")).toBeUndefined();
    expect(ran).toEqual(["read"]);
  });
});

describe("the window's key listener", () => {
  it("runs the keymap's command and takes the press, and leaves presses nothing handles", () => {
    const r = new CommandRegistry();
    const ran: string[] = [];
    r.register({ id: "workspace:new-tab", name: "New tab", run: () => void ran.push("new-tab") });
    const off = installKeyListener(r);
    expect(press(document.body, "t", { ctrlKey: true }).defaultPrevented).toBe(true);
    expect(press(document.body, "p", { ctrlKey: true }).defaultPrevented).toBe(false); // no command registered
    expect(press(document.body, "t", { ctrlKey: true, metaKey: true }).defaultPrevented).toBe(false);
    off();
    press(document.body, "t", { ctrlKey: true });
    expect(ran).toEqual(["new-tab"]);
  });

  it("reads Shift+Tab as WebKitGTK names it, Unidentified with the code Tab", () => {
    const r = new CommandRegistry();
    const ran: string[] = [];
    r.register({ id: "workspace:previous-tab", name: "Go to previous tab", run: () => void ran.push("previous") });
    const off = installKeyListener(r);
    const e = press(document.body, "Unidentified", { code: "Tab", ctrlKey: true, shiftKey: true });
    press(document.body, "Unidentified", { code: "KeyQ", ctrlKey: true, shiftKey: true });
    off();
    expect(e.defaultPrevented).toBe(true);
    expect(ran).toEqual(["previous"]);
  });

  it("skips handled presses and, in a field outside the editor, presses without Ctrl or Alt", () => {
    const r = new CommandRegistry();
    const ran: string[] = [];
    r.register(
      { id: "workspace:edit-file-title", name: "Rename file", run: () => void ran.push("F2") },
      { id: "workspace:new-tab", name: "New tab", run: () => void ran.push("Ctrl+T") },
      { id: "app:go-back", name: "Navigate back", run: () => void ran.push("back") },
    );
    const off = installKeyListener(r);
    document.body.innerHTML = `<input id="field"><div class="cm-editor"><div contenteditable="true" id="cm"></div></div>`;
    const field = document.getElementById("field") as HTMLElement;
    const cm = document.getElementById("cm") as HTMLElement;
    press(field, "F2");
    press(field, "t", { ctrlKey: true });
    press(cm, "ArrowLeft", { ctrlKey: true, altKey: true });
    const handled = new KeyboardEvent("keydown", { key: "t", ctrlKey: true, bubbles: true, cancelable: true });
    field.addEventListener("keydown", (e) => e.preventDefault(), { once: true });
    field.dispatchEvent(handled);
    off();
    expect(ran).toEqual(["F2", "Ctrl+T", "back"]);
  });
});
