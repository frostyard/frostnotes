// Where the on-screen keyboard is, for a narrow window
// (docs/specs/ui.md#narrow-windows): which kind of text field has the
// focus, which is what opens the keyboard on a touch screen, and how far
// the visual viewport's bottom sits above the layout viewport's, which is
// the keyboard's height where a browser shrinks the visual viewport for
// it.
import { useEffect, useLayoutEffect, useState } from "react";

/** TextFocus is what has the focus: a note's editor, another text field, or neither. */
export type TextFocus = "editor" | "field" | null;

// Inputs that take no typed text, and so open no on-screen keyboard.
const NO_TEXT = new Set([
  "button",
  "checkbox",
  "color",
  "file",
  "hidden",
  "image",
  "radio",
  "range",
  "reset",
  "submit",
]);

/** textFocusOf classifies the element that has the focus. */
export function textFocusOf(el: Element | null): TextFocus {
  if (!el?.isConnected) return null;
  // A field an editor draws (the inline title, a property, its search) is a
  // field, not the editor: the editing toolbar acts on the editor's text.
  if (el instanceof HTMLTextAreaElement) return el.readOnly ? null : "field";
  if (el instanceof HTMLInputElement) return el.readOnly || NO_TEXT.has(el.type) ? null : "field";
  if (el.closest(".cm-editor")) return "editor";
  if (el instanceof HTMLElement && el.isContentEditable) return "field";
  return null;
}

/** useTextFocus reports which kind of text field has the focus. */
export function useTextFocus(): TextFocus {
  const [focus, setFocus] = useState<TextFocus>(null);
  useEffect(() => {
    // A field that leaves the document (a prompt that closes) may take the
    // focus with it without a focusout: watch for that while one has it.
    const gone = new MutationObserver(() => {
      if (!document.activeElement?.isConnected || document.activeElement === document.body) check();
    });
    const check = () => {
      const kind = textFocusOf(document.activeElement);
      setFocus(kind);
      gone.disconnect();
      if (kind !== null) gone.observe(document.body, { childList: true, subtree: true });
    };
    // Focus moves after focusout: look once it has landed.
    let timer: ReturnType<typeof setTimeout> | undefined;
    const out = () => {
      clearTimeout(timer);
      timer = setTimeout(check, 0);
    };
    document.addEventListener("focusin", check);
    document.addEventListener("focusout", out);
    check();
    return () => {
      clearTimeout(timer);
      gone.disconnect();
      document.removeEventListener("focusin", check);
      document.removeEventListener("focusout", out);
    };
  }, []);
  return focus;
}

/** useKeyboardInset is the keyboard's height over the layout viewport, 0 where there is none. */
export function useKeyboardInset(): number {
  const [inset, setInset] = useState(0);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const update = () => setInset(Math.max(0, Math.round(window.innerHeight - (vv.offsetTop + vv.height))));
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    update();
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
    };
  }, []);
  return inset;
}

/**
 * usePromptsEndAtKeyboard makes the open prompts (lodge-ui's Prompt: a
 * modal dialog led by its query field) end at the keyboard, inset high
 * above the layout viewport's bottom. lodge-ui gives a narrow prompt
 * 100dvh, which iOS Safari does not shrink for the keyboard. open names
 * what is open, so that a prompt that opens is fitted too.
 */
export function usePromptsEndAtKeyboard(inset: number, open: string | null): void {
  // biome-ignore lint/correctness/useExhaustiveDependencies: open is the trigger
  useLayoutEffect(() => {
    const prompts = [...document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]')].filter(
      (d) => d.firstElementChild?.matches('input[role="combobox"]') === true,
    );
    for (const d of prompts) d.style.maxHeight = inset > 0 ? `calc(100dvh - ${inset}px)` : "";
  }, [inset, open]);
}
