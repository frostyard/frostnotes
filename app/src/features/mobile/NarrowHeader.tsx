// The view header of a narrow window (docs/specs/ui.md#narrow-windows): 44
// high, as Obsidian's phone header, Open left sidebar at the left, the
// view's title in the middle, and Reading or Editing view, More options
// and Open right sidebar at the right. A tap on the title shows its
// folders, as the breadcrumb does; a tap on one reveals it. Back and
// forward are the bottom bar's.
import { BookOpen, Ellipsis, PanelLeft, PanelRight, Pencil } from "lucide-react";
import { Fragment, useState } from "react";

import { TouchButton } from "./TouchButton";

export interface NarrowHeaderProps {
  title: string;
  /** The file's path, whose folders a tap on the title shows; absent for views without a file. */
  path?: string;
  /** A note's mode; absent for other views, which have no mode button. */
  mode?: "edit" | "read";
  onLeft: () => void;
  onRight: () => void;
  onToggleMode: () => void;
  onMore: (x: number, y: number) => void;
  /** A folder the title showed was tapped: reveal it. */
  onFolder?: (folder: string) => void;
}

/** NarrowHeader draws a narrow window's view header. */
export function NarrowHeader(props: NarrowHeaderProps) {
  const [folders, setFolders] = useState(false);
  const parts = props.path?.split("/").slice(0, -1) ?? [];
  const title = <span className="truncate">{props.title}</span>;
  return (
    <div className="flex h-11 shrink-0 items-center gap-1 border-b border-separator bg-window px-1">
      <TouchButton label="Open left sidebar" icon={PanelLeft} onClick={props.onLeft} />
      <h1 className="flex min-w-0 flex-1 justify-center text-toolbar-title">
        {parts.length === 0 ? (
          title
        ) : folders ? (
          <nav aria-label="Breadcrumb" className="flex min-w-0 items-center">
            {parts.map((name, i) => {
              const folder = parts.slice(0, i + 1).join("/");
              return (
                <Fragment key={folder}>
                  <button
                    type="button"
                    className="min-h-11 shrink-0 px-1 text-secondary"
                    onClick={() => props.onFolder?.(folder)}
                  >
                    {name}
                  </button>
                  <span className="shrink-0 text-tertiary">/</span>
                </Fragment>
              );
            })}
            <button type="button" className="min-h-11 min-w-0 truncate px-1" onClick={() => setFolders(false)}>
              {props.title}
            </button>
          </nav>
        ) : (
          <button
            type="button"
            className="min-h-11 min-w-0 truncate px-1"
            aria-label={`${props.title}: show its folders`}
            onClick={() => setFolders(true)}
          >
            {props.title}
          </button>
        )}
      </h1>
      {props.mode !== undefined && (
        <TouchButton
          label={props.mode === "edit" ? "Reading view" : "Editing view"}
          icon={props.mode === "edit" ? BookOpen : Pencil}
          onClick={props.onToggleMode}
        />
      )}
      <TouchButton
        label="More options"
        icon={Ellipsis}
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          props.onMore(r.left, r.bottom);
        }}
      />
      <TouchButton label="Open right sidebar" icon={PanelRight} onClick={props.onRight} />
    </div>
  );
}
