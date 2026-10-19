// The view header of a narrow window (docs/specs/ui.md#narrow-windows): 52
// high, Open left sidebar at the left, the view's title in the middle, and
// Reading or Editing view, More options and Open right sidebar at the
// right. Back and forward are the bottom bar's.
import { BookOpen, Ellipsis, PanelLeft, PanelRight, Pencil } from "lucide-react";

import { TouchButton } from "./TouchButton";

export interface NarrowHeaderProps {
  title: string;
  /** A note's mode; absent for other views, which have no mode button. */
  mode?: "edit" | "read";
  onLeft: () => void;
  onRight: () => void;
  onToggleMode: () => void;
  onMore: (x: number, y: number) => void;
}

/** NarrowHeader draws a narrow window's view header. */
export function NarrowHeader(props: NarrowHeaderProps) {
  return (
    <div className="flex h-[52px] shrink-0 items-center gap-1 border-b border-separator bg-window px-1">
      <TouchButton label="Open left sidebar" icon={<PanelLeft size={20} />} onClick={props.onLeft} />
      <h1 className="min-w-0 flex-1 truncate text-center text-toolbar-title">{props.title}</h1>
      {props.mode !== undefined && (
        <TouchButton
          label={props.mode === "edit" ? "Reading view" : "Editing view"}
          icon={props.mode === "edit" ? <BookOpen size={20} /> : <Pencil size={20} />}
          onClick={props.onToggleMode}
        />
      )}
      <TouchButton
        label="More options"
        icon={<Ellipsis size={20} />}
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          props.onMore(r.left, r.bottom);
        }}
      />
      <TouchButton label="Open right sidebar" icon={<PanelRight size={20} />} onClick={props.onRight} />
    </div>
  );
}
