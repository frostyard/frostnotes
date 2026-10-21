// Vitest setup for component tests (happy-dom, Testing Library). Globals are
// off, so cleanup is registered here.
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library prints the DOM with a failed query; keep it short enough
// that the failure itself stays in view.
process.env.DEBUG_PRINT_LIMIT ??= "1500";

configure({ asyncUtilTimeout: 5000 });

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
