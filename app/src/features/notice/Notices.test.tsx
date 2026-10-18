import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { NOTICE_ACTION_MS, NOTICE_MS, type NoticeItem, Notices } from "./Notices";

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

const A: NoticeItem = { id: "a", text: "Moved Garden plan to Projects." };
const B: NoticeItem = { id: "b", text: "Deleted Inbox.", action: { label: "Undo", run: vi.fn() } };

describe("Notices", () => {
  it("stacks the notices in order, each a status", () => {
    render(<Notices notices={[A, B]} onDismiss={vi.fn()} />);
    const shown = screen.getAllByRole("status");
    expect(shown.map((s) => s.textContent)).toEqual(["Moved Garden plan to Projects.", "Deleted Inbox.Undo"]);
    expect(shown[0]?.className).toContain("bg-toolbar");
    expect(shown[0]?.className).toContain("shadow-popover");
    expect(shown[0]?.parentElement?.className).toContain("fixed");
  });

  it("dismisses a notice after its time: longer with an action", () => {
    const onDismiss = vi.fn();
    render(<Notices notices={[A, B]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(NOTICE_MS - 1));
    expect(onDismiss).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(onDismiss.mock.calls).toEqual([["a"]]);
    act(() => vi.advanceTimersByTime(NOTICE_ACTION_MS - NOTICE_MS));
    expect(onDismiss.mock.calls).toEqual([["a"], ["b"]]);
    expect(NOTICE_MS).toBe(4000);
    expect(NOTICE_ACTION_MS).toBe(10000);
  });

  it("times each notice from when it first shows, not from each render", () => {
    const onDismiss = vi.fn();
    const view = render(<Notices notices={[A]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(3000));
    const C: NoticeItem = { id: "c", text: "Copied the path." };
    view.rerender(<Notices notices={[A, C]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(1000));
    expect(onDismiss.mock.calls).toEqual([["a"]]);
    view.rerender(<Notices notices={[C]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(3000));
    expect(onDismiss.mock.calls).toEqual([["a"], ["c"]]);
  });

  it("dismisses a notice once, however often the stack draws again", () => {
    const onDismiss = vi.fn();
    const view = render(<Notices notices={[A]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(3000));
    view.rerender(<Notices notices={[{ ...A }]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(NOTICE_ACTION_MS));
    expect(onDismiss.mock.calls).toEqual([["a"]]);
  });

  it("forgets the timer of a notice that left early", () => {
    const onDismiss = vi.fn();
    const view = render(<Notices notices={[A]} onDismiss={onDismiss} />);
    view.rerender(<Notices notices={[]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(NOTICE_ACTION_MS));
    expect(onDismiss).not.toHaveBeenCalled();
    view.rerender(<Notices notices={[A]} onDismiss={onDismiss} />);
    act(() => vi.advanceTimersByTime(NOTICE_MS));
    expect(onDismiss.mock.calls).toEqual([["a"]]);
  });

  it("dismisses a clicked notice", () => {
    const onDismiss = vi.fn();
    render(<Notices notices={[A]} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByText("Moved Garden plan to Projects."));
    expect(onDismiss.mock.calls).toEqual([["a"]]);
  });

  it("runs an action, then dismisses its notice once", () => {
    const onDismiss = vi.fn();
    const run = vi.fn();
    render(<Notices notices={[{ ...B, action: { label: "Undo", run } }]} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(run).toHaveBeenCalledTimes(1);
    expect(onDismiss.mock.calls).toEqual([["b"]]);
  });

  it("stops its timers when it goes", () => {
    const onDismiss = vi.fn();
    const view = render(<Notices notices={[A, B]} onDismiss={onDismiss} />);
    view.unmount();
    act(() => vi.advanceTimersByTime(NOTICE_ACTION_MS));
    expect(onDismiss).not.toHaveBeenCalled();
  });
});
