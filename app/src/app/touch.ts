// Touch for a phone (docs/specs/ui.md#narrow-windows): a long press opens
// the context menu a right click would, and a horizontal swipe from a
// screen edge opens that side's drawer. Both listen on the window, and
// leave the editor's own touch (selecting text) alone.

/** LONG_PRESS_MS is how long a touch rests before it is a long press. */
export const LONG_PRESS_MS = 500;
/** EDGE_PX is how near a screen edge a swipe that opens a drawer starts. */
export const EDGE_PX = 24;
/** SWIPE_PX is how far a touch moves before it is a swipe, and a long press is not. */
export const SWIPE_PX = 10;
const OPEN_PX = 50;

/** Target is the part of window the handlers use; the page's window by default. */
export type Target = Pick<Window, "addEventListener" | "removeEventListener" | "innerWidth">;

/**
 * installLongPress turns a touch that rests LONG_PRESS_MS without moving
 * into a contextmenu event at its point, which the app's menus answer,
 * and swallows the click that ends it. It returns the removal.
 */
export function installLongPress(target: Target = window, delay = LONG_PRESS_MS): () => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let start: { x: number; y: number; el: Element } | null = null;
  let fired = false;
  const cancel = () => {
    clearTimeout(timer);
    start = null;
  };
  const down = (e: Event) => {
    const p = e as PointerEvent;
    if (p.pointerType !== "touch" || !(p.target instanceof Element) || p.target.closest(".cm-editor")) return;
    fired = false;
    start = { x: p.clientX, y: p.clientY, el: p.target };
    const at = start;
    timer = setTimeout(() => {
      fired = true;
      at.el.dispatchEvent(
        new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: at.x, clientY: at.y, button: 2 }),
      );
      start = null;
    }, delay);
  };
  const move = (e: Event) => {
    const p = e as PointerEvent;
    if (start && Math.hypot(p.clientX - start.x, p.clientY - start.y) > SWIPE_PX) cancel();
  };
  const click = (e: Event) => {
    if (!fired) return;
    fired = false;
    e.preventDefault();
    e.stopPropagation();
  };
  target.addEventListener("pointerdown", down, true);
  target.addEventListener("pointermove", move, true);
  target.addEventListener("pointerup", cancel, true);
  target.addEventListener("pointercancel", cancel, true);
  target.addEventListener("click", click, true);
  return () => {
    cancel();
    target.removeEventListener("pointerdown", down, true);
    target.removeEventListener("pointermove", move, true);
    target.removeEventListener("pointerup", cancel, true);
    target.removeEventListener("pointercancel", cancel, true);
    target.removeEventListener("click", click, true);
  };
}

/**
 * installEdgeSwipe calls open("left") or open("right") for a touch that
 * starts within EDGE_PX of that screen edge and moves at least 50 pixels
 * inward, more across than down. It returns the removal.
 */
export function installEdgeSwipe(open: (side: "left" | "right") => void, target: Target = window): () => void {
  let start: { x: number; y: number; side: "left" | "right" } | null = null;
  const down = (e: Event) => {
    const p = e as PointerEvent;
    if (p.pointerType !== "touch") return;
    const side = p.clientX <= EDGE_PX ? "left" : p.clientX >= target.innerWidth - EDGE_PX ? "right" : null;
    start = side ? { x: p.clientX, y: p.clientY, side } : null;
  };
  const move = (e: Event) => {
    const p = e as PointerEvent;
    if (!start) return;
    const dx = p.clientX - start.x;
    const dy = Math.abs(p.clientY - start.y);
    const inward = start.side === "left" ? dx : -dx;
    if (inward >= OPEN_PX && inward > dy) {
      open(start.side);
      start = null;
    }
  };
  const up = () => {
    start = null;
  };
  target.addEventListener("pointerdown", down, true);
  target.addEventListener("pointermove", move, true);
  target.addEventListener("pointerup", up, true);
  target.addEventListener("pointercancel", up, true);
  return () => {
    target.removeEventListener("pointerdown", down, true);
    target.removeEventListener("pointermove", move, true);
    target.removeEventListener("pointerup", up, true);
    target.removeEventListener("pointercancel", up, true);
  };
}
