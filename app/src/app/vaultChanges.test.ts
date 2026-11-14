import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { applyFileEvent, emptyFiles, fromListing, useFiles } from "../data/files";
import { LINKS_DELAY, useLinksVersion } from "./vaultChanges";

afterEach(() => {
  vi.useRealTimers();
  useFiles.setState(emptyFiles(null));
});

const NOW = () => "2026-11-14T12:00:00.000Z";

// event applies a note event to the files store, as notesd's events do.
function event(e: Parameters<typeof applyFileEvent>[1]) {
  act(() => useFiles.setState((s) => applyFileEvent(s, e, NOW)));
}
const saved = (path: string, hash: string) =>
  event({ event: "note.changed", data: { vaultId: 3, path, hash, source: "editor" } });

describe("useLinksVersion", () => {
  it("goes up once the saves stop, not at each save", () => {
    vi.useFakeTimers();
    useFiles.setState(
      fromListing(
        3,
        { files: [{ path: "a.md", kind: "note", hash: "h0", size: 1, modifiedAt: NOW() }], folders: [] },
        0,
      ),
    );
    const { result } = renderHook(() => useLinksVersion());
    expect(result.current).toBe(0);
    saved("a.md", "h1");
    act(() => vi.advanceTimersByTime(LINKS_DELAY - 1));
    saved("a.md", "h2");
    act(() => vi.advanceTimersByTime(LINKS_DELAY - 1));
    saved("a.md", "h3");
    act(() => vi.advanceTimersByTime(LINKS_DELAY - 1));
    expect(result.current).toBe(0);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe(1);
    act(() => vi.advanceTimersByTime(10 * LINKS_DELAY));
    expect(result.current).toBe(1);
  });

  it("goes up at once when the set of paths changes, with the saves before it", () => {
    vi.useFakeTimers();
    useFiles.setState(
      fromListing(
        3,
        { files: [{ path: "a.md", kind: "note", hash: "h0", size: 1, modifiedAt: NOW() }], folders: [] },
        0,
      ),
    );
    const { result } = renderHook(() => useLinksVersion());
    saved("a.md", "h1");
    saved("b.md", "h1"); // a new file can change where a link leads
    expect(result.current).toBe(1);
    event({ event: "note.moved", data: { vaultId: 3, from: "b.md", to: "c.md" } });
    expect(result.current).toBe(2);
    event({ event: "note.removed", data: { vaultId: 3, path: "c.md" } });
    expect(result.current).toBe(3);
    act(() => vi.advanceTimersByTime(10 * LINKS_DELAY));
    expect(result.current).toBe(3);
  });
});
