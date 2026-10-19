// Where the on-screen keyboard is, for the narrow window's editing
// toolbar (docs/specs/ui.md#narrow-windows): whether an editor has the
// focus, and how far the visual viewport's bottom sits above the layout
// viewport's, which is the keyboard's height where a browser shrinks the
// visual viewport for it.
import { useEffect, useState } from "react";

/** useEditorFocus reports whether a note's editor has the focus. */
export function useEditorFocus(): boolean {
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    const check = () => setFocused(document.activeElement?.closest(".cm-editor") != null);
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
      document.removeEventListener("focusin", check);
      document.removeEventListener("focusout", out);
    };
  }, []);
  return focused;
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
