// A view's header (docs/specs/ui.md#view-header): back and forward, the
// breadcrumb of the file's folders, and the mode switch and More options.
// The container decides what each does; More options asks for a menu at
// the button.
import { ArrowLeft, ArrowRight, BookOpen, Ellipsis, Pencil } from "lucide-react";
import { Fragment } from "react";

import { IconButton } from "../chrome/IconButton";

export interface ViewHeaderProps {
  /** The file's vault path; none for an empty tab. */
  path?: string;
  /** The title after the folders. */
  title: string;
  canBack: boolean;
  canForward: boolean;
  /** The note's mode, when the view shows a note. */
  mode?: "edit" | "read";
  onBack: () => void;
  onForward: () => void;
  /** A folder of the breadcrumb was clicked: reveal it in the Files pane. */
  onFolder: (folder: string) => void;
  onToggleMode: () => void;
  /** Open More options at (x, y), in client coordinates. */
  onMore: (x: number, y: number) => void;
}

/** ViewHeader draws a view's header. */
export function ViewHeader(props: ViewHeaderProps) {
  const parts = props.path?.split("/").slice(0, -1) ?? [];
  return (
    <div className="text-tab flex h-9 shrink-0 items-center gap-1 bg-window px-2">
      <IconButton
        size={24}
        label="Navigate back"
        keys="Ctrl + Alt + ←"
        icon={<ArrowLeft size={16} />}
        disabled={!props.canBack}
        onClick={props.onBack}
      />
      <IconButton
        size={24}
        label="Navigate forward"
        keys="Ctrl + Alt + →"
        icon={<ArrowRight size={16} />}
        disabled={!props.canForward}
        onClick={props.onForward}
      />
      <nav aria-label="Breadcrumb" className="flex min-w-0 flex-1 items-center justify-center truncate">
        {parts.map((name, i) => {
          const folder = parts.slice(0, i + 1).join("/");
          return (
            <Fragment key={folder}>
              <button
                type="button"
                className="shrink-0 text-secondary hover:text-primary"
                onClick={() => props.onFolder(folder)}
              >
                {name}
              </button>
              <span className="shrink-0 px-1 text-tertiary">/</span>
            </Fragment>
          );
        })}
        <span className="truncate text-primary">{props.title}</span>
      </nav>
      {props.mode && (
        <IconButton
          size={24}
          label={props.mode === "edit" ? "Reading view" : "Editing view"}
          keys="Ctrl + E"
          icon={props.mode === "edit" ? <BookOpen size={16} /> : <Pencil size={16} />}
          onClick={props.onToggleMode}
        />
      )}
      <IconButton
        size={24}
        label="More options"
        icon={<Ellipsis size={16} />}
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          props.onMore(r.left, r.bottom);
        }}
      />
    </div>
  );
}
