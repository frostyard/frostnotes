// A link's page preview (docs/specs/ui.md, Keyboard map: Mouse). The
// container controls its visibility and fills the scrollable note body.
import { useEffect, useLayoutEffect, useRef } from "react";

/** PREVIEW_WIDTH and PREVIEW_HEIGHT are the preview's size at most, in px. */
export const PREVIEW_WIDTH = 400;
export const PREVIEW_HEIGHT = 300;

export interface PagePreviewProps {
  /** The link's box on screen. */
  anchor: { left: number; top: number; bottom: number };
  title: string;
  /** Fills the body with the note. */
  fill: (el: HTMLElement) => void;
  onOpen: () => void;
  onClose: () => void;
}

/** previewPosition places the preview inside the window, 8px from its edges, 4px from the link. */
export function previewPosition(
  anchor: PagePreviewProps["anchor"],
  width: number,
  height: number,
): { left: number; top: number } {
  return {
    left: Math.max(8, Math.min(anchor.left, width - PREVIEW_WIDTH - 8)),
    top:
      anchor.bottom + 4 + PREVIEW_HEIGHT <= height - 8
        ? anchor.bottom + 4
        : Math.max(8, anchor.top - 4 - PREVIEW_HEIGHT),
  };
}

/** PagePreview draws a link's page preview. */
export function PagePreview({ anchor, title, fill, onOpen, onClose }: PagePreviewProps) {
  const box = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (body.current) fill(body.current);
  }, [fill]);

  useEffect(() => {
    function keyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    function outside(event: MouseEvent) {
      if (event.target instanceof Node && !box.current?.contains(event.target)) onClose();
    }
    window.addEventListener("keydown", keyDown);
    document.addEventListener("mousedown", outside);
    return () => {
      window.removeEventListener("keydown", keyDown);
      document.removeEventListener("mousedown", outside);
    };
  }, [onClose]);

  return (
    <div
      ref={box}
      role="dialog"
      aria-label={`Preview of ${title}`}
      className="fixed z-50 flex flex-col overflow-hidden rounded-lg border border-separator bg-window shadow-popover"
      style={{
        ...previewPosition(anchor, window.innerWidth, window.innerHeight),
        width: PREVIEW_WIDTH,
        maxHeight: PREVIEW_HEIGHT,
      }}
      onMouseLeave={onClose}
    >
      <button
        type="button"
        className="text-toolbar-title shrink-0 truncate border-b border-separator px-3 py-2 text-left"
        onClick={() => onOpen()}
      >
        {title}
      </button>
      <div ref={body} className="rendered min-h-0 overflow-auto p-3" />
    </div>
  );
}
