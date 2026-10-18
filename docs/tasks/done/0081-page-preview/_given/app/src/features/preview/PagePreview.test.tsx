// T-0081's given tests: a link's page preview, its place on screen, its
// body and how it closes.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { PagePreview, type PagePreviewProps, PREVIEW_HEIGHT, PREVIEW_WIDTH, previewPosition } from "./PagePreview";

function setup(over: Partial<PagePreviewProps> = {}) {
  const props: PagePreviewProps = {
    anchor: { left: 100, top: 80, bottom: 100 },
    title: "Plan",
    fill: vi.fn((el: HTMLElement) => {
      el.textContent = "Rows of quince.";
    }),
    onOpen: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(
    <>
      <PagePreview {...props} />
      <p>outside</p>
    </>,
  );
  return props;
}

const preview = () => screen.getByRole("dialog", { name: "Preview of Plan" });

describe("previewPosition", () => {
  it("puts the preview 4px under the link", () => {
    expect(previewPosition({ left: 100, top: 80, bottom: 100 }, 1024, 768)).toEqual({ left: 100, top: 104 });
  });

  it("keeps it 8px inside the window's right edge and left edge", () => {
    expect(previewPosition({ left: 900, top: 80, bottom: 100 }, 1024, 768).left).toBe(1024 - PREVIEW_WIDTH - 8);
    expect(previewPosition({ left: 2, top: 80, bottom: 100 }, 1024, 768).left).toBe(8);
  });

  it("puts it above the link when there is no room below, never above the window", () => {
    expect(previewPosition({ left: 100, top: 600, bottom: 620 }, 1024, 768).top).toBe(600 - 4 - PREVIEW_HEIGHT);
    expect(previewPosition({ left: 100, top: 200, bottom: 220 }, 1024, 400).top).toBe(8);
  });
});

describe("PagePreview", () => {
  it("shows the note's title, which opens it, and lets the container fill its body", () => {
    const props = setup();
    expect(props.fill).toHaveBeenCalledTimes(1);
    expect(preview().textContent).toContain("Rows of quince.");
    expect((props.fill as ReturnType<typeof vi.fn>).mock.calls[0]?.[0]?.classList.contains("rendered")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Plan" }));
    expect(props.onOpen).toHaveBeenCalledTimes(1);
  });

  it("sits where previewPosition says, at most its size", () => {
    setup();
    const style = preview().style;
    expect([style.left, style.top, style.width, style.maxHeight]).toEqual([
      "100px",
      "104px",
      `${PREVIEW_WIDTH}px`,
      `${PREVIEW_HEIGHT}px`,
    ]);
  });

  it("closes on Escape, a press outside it and the pointer leaving it, not on a press inside", () => {
    const props = setup();
    fireEvent.mouseDown(screen.getByRole("button", { name: "Plan" }));
    expect(props.onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.mouseDown(screen.getByText("outside"));
    fireEvent.mouseLeave(preview());
    expect(props.onClose).toHaveBeenCalledTimes(3);
  });
});
