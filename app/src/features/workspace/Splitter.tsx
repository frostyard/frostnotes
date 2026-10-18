// A splitter (docs/specs/ui.md#layout): a 1px separator line with a 6px
// invisible drag handle that turns accent while dragged. It reports how far
// it has moved since the drag began; arrow keys move it 16px at a time.
import { type KeyboardEvent, type PointerEvent, useRef, useState } from "react";

export interface SplitterProps {
  /** "vertical" separates side-by-side panes and drags along x. */
  orientation: "vertical" | "horizontal";
  label: string;
  /** Where it stands, as a percentage of the space it divides (aria-valuenow). */
  value: number;
  /** The drag began. */
  onStart: () => void;
  /** It moved by delta pixels since the drag began (or the key press). */
  onMove: (delta: number) => void;
  onEnd?: () => void;
}

/** Splitter draws a drag handle between panes. */
export function Splitter(props: SplitterProps) {
  const start = useRef<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const vertical = props.orientation === "vertical";
  const coord = (e: PointerEvent) => (vertical ? e.clientX : e.clientY);

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture?.(e.pointerId);
    start.current = coord(e);
    setDragging(true);
    props.onStart();
  };
  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    if (start.current !== null) props.onMove(coord(e) - start.current);
  };
  const onPointerUp = () => {
    if (start.current === null) return;
    start.current = null;
    setDragging(false);
    props.onEnd?.();
  };
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const steps: Record<string, number> = vertical
      ? { ArrowLeft: -16, ArrowRight: 16 }
      : { ArrowUp: -16, ArrowDown: 16 };
    const step = steps[e.key];
    if (step === undefined) return;
    e.preventDefault();
    props.onStart();
    props.onMove(step);
    props.onEnd?.();
  };

  return (
    // biome-ignore lint/a11y/useSemanticElements: a focusable separator is the ARIA pattern for a splitter; <hr> takes no focus
    <div
      role="separator"
      aria-orientation={props.orientation}
      aria-label={props.label}
      aria-valuenow={Math.round(props.value)}
      aria-valuemin={0}
      aria-valuemax={100}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={onKeyDown}
      className={`group relative shrink-0 focus-visible:outline-none ${vertical ? "w-px cursor-col-resize" : "h-px cursor-row-resize"} ${
        dragging ? "bg-accent" : "bg-separator focus-visible:bg-accent"
      }`}
    >
      <div className={`absolute z-10 ${vertical ? "inset-y-0 -left-[3px] w-1.5" : "inset-x-0 -top-[3px] h-1.5"}`} />
    </div>
  );
}
