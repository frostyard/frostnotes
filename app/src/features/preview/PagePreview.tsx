// A link's page preview (docs/specs/ui.md, Keyboard map: Mouse). Task
// T-0081 replaces this stub.

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
  _width: number,
  _height: number,
): { left: number; top: number } {
  return { left: anchor.left, top: anchor.bottom };
}

/** PagePreview draws a link's page preview. */
export function PagePreview(_props: PagePreviewProps) {
  return null;
}
