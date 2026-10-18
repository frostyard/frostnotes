// A view that fails while it draws shows why in its tab; the rest of the
// window works on (review finding 6, plan 0005 Phase 7).
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ViewBoundary } from "./ViewBoundary";

function Broken(): never {
  throw new RangeError("Maximum call stack size exceeded");
}

describe("ViewBoundary", () => {
  it("shows a failed view's error in its place, keeps the rest, and draws the next view afresh", () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    const { rerender } = render(
      <div>
        <ViewBoundary label="Plan.md" view="t1:note:Plan.md">
          <Broken />
        </ViewBoundary>
        <p>Still here</p>
      </div>,
    );
    expect(screen.getByRole("alert").textContent).toBe("Cannot show Plan.md: Maximum call stack size exceeded");
    expect(screen.getByText("Still here")).toBeTruthy();
    rerender(
      <div>
        <ViewBoundary label="Other.md" view="t1:note:Other.md">
          <p>Other</p>
        </ViewBoundary>
        <p>Still here</p>
      </div>,
    );
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByText("Other")).toBeTruthy();
    quiet.mockRestore();
  });
});
