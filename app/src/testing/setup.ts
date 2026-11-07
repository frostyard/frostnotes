// Vitest setup for component tests (happy-dom, Testing Library). Globals are
// off, so cleanup is registered here.
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library prints the DOM with a failed query; keep it short enough
// that the failure itself stays in view.
process.env.DEBUG_PRINT_LIMIT ??= "1500";

configure({ asyncUtilTimeout: 5000 });

// happy-dom 21 fires `selectionchange` at once, inside the call that changed
// the selection, and again when a new range holds the same points; a browser
// queues it, and fires it only for a change
// (https://w3c.github.io/selection-api/#selectionchange-event). Fired at once,
// it reaches CodeMirror while the view is inside an update and starts a second
// one, which CodeMirror refuses. The test DOM queues it, as a browser does,
// and leaves out a selection that has not moved since the last one it fired.
// happy-dom's own dispatch calls `dispatchEvent` again on each target of the
// event's path, so the event that is being delivered passes straight through.
if (typeof document !== "undefined") {
  const dispatchEvent = document.dispatchEvent.bind(document);
  let fired = "";
  let queued = false;
  let delivering: Event | null = null;
  const nodes = new WeakMap<Node, number>();
  let next = 0;
  const id = (n: Node | null) => {
    if (!n) return 0;
    let v = nodes.get(n);
    if (v === undefined) {
      next += 1;
      v = next;
      nodes.set(n, v);
    }
    return v;
  };
  const key = () => {
    const s = document.getSelection();
    if (!s || s.rangeCount === 0) return "none";
    return `${id(s.anchorNode)}:${s.anchorOffset}:${id(s.focusNode)}:${s.focusOffset}`;
  };
  document.dispatchEvent = (event: Event): boolean => {
    if (event.type !== "selectionchange" || event === delivering) return dispatchEvent(event);
    if (queued) return true;
    queued = true;
    queueMicrotask(() => {
      queued = false;
      const now = key();
      if (now === fired) return;
      fired = now;
      delivering = event;
      try {
        dispatchEvent(event);
      } finally {
        delivering = null;
      }
    });
    return true;
  };
}

// jsdom has no matchMedia, and the window reads it for its narrow layout:
// a desktop window, never narrow, is enough for the tests that run there.
if (typeof window !== "undefined" && typeof window.matchMedia !== "function") {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

afterEach(cleanup);
