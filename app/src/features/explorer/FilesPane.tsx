// The Files pane (docs/specs/ui.md, Files pane): its toolbar and the
// vault's tree, with keyboard navigation, inline renaming and drag targets.

import { IconButton } from "@frostyard/lodge-ui/react";
import { ArrowUpNarrowWide, ChevronRight, ChevronsDownUp, ChevronsUpDown, FolderPlus, SquarePen } from "lucide-react";
import { type KeyboardEvent, useEffect, useRef, useState } from "react";
import type { TreeRow } from "../../lib/fileTree";
import { type PaneClick, paneClick, paneToolbar } from "../panes/common";

/** DRAG_TYPE is the data type a dragged row carries: a JSON array of vault paths. */
export const DRAG_TYPE = "application/x-frostnotes-paths";

/** FilesCommand is what the pane asks of its container. */
export type FilesCommand =
  /** Open a file: a click, a middle click, or Enter. */
  | { kind: "open"; path: string; click: PaneClick }
  /** Open or close a folder. */
  | { kind: "toggle"; path: string }
  | { kind: "newNote" }
  | { kind: "newFolder" }
  /** Open the sort order's menu at (x, y). */
  | { kind: "sortMenu"; x: number; y: number }
  | { kind: "collapseAll" }
  | { kind: "expandAll" }
  /** Open the context menu of a row, or of the vault's root (path null), at (x, y). */
  | { kind: "menu"; path: string | null; folder: boolean; x: number; y: number }
  /** Start renaming a row (F2). */
  | { kind: "rename"; path: string }
  /** Rename a row to name, as typed (a file's name without its extension). */
  | { kind: "renamed"; path: string; name: string }
  | { kind: "cancelRename" }
  /** Move the dragged paths into folder ("" for the vault's root). */
  | { kind: "drop"; paths: string[]; folder: string };

export interface FilesPaneProps {
  rows: readonly TreeRow[];
  /** The active tab's file. */
  active: string | null;
  /** Some folder is open: the toolbar offers Collapse all, else Expand all. */
  anyExpanded: boolean;
  /** Draw the indentation guides. */
  indentGuides: boolean;
  /** The row whose name is being edited. */
  renaming: string | null;
  /** The listing has arrived: only then does an empty list mean an empty vault. */
  loaded: boolean;
  onCommand: (command: FilesCommand) => void;
}

/** FilesPane draws the Files pane. */
export function FilesPane(props: FilesPaneProps) {
  const { rows, active, renaming, onCommand } = props;
  const elements = useRef(new Map<string, HTMLDivElement>());
  const [focused, setFocused] = useState<string | null>(null);
  const [dropFolder, setDropFolder] = useState<string | null>(null);
  const tabPath =
    rows.find((row) => row.path === focused)?.path ?? rows.find((row) => row.path === active)?.path ?? rows[0]?.path;

  useEffect(() => {
    if (active !== null) elements.current.get(active)?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  function focus(row: TreeRow | undefined) {
    if (!row) return;
    setFocused(row.path);
    elements.current.get(row.path)?.focus();
  }

  function eventRow(target: EventTarget) {
    const path = target instanceof Element ? target.closest("[data-path]")?.getAttribute("data-path") : null;
    return rows.find((row) => row.path === path);
  }

  function targetFolder(target: EventTarget) {
    const row = eventRow(target);
    if (!row) return "";
    return row.kind === "folder" ? row.path : row.path.slice(0, Math.max(0, row.path.lastIndexOf("/")));
  }

  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.target instanceof HTMLInputElement) return;
    const row = eventRow(event.target);
    if (!row) return;
    const index = rows.indexOf(row);
    switch (event.key) {
      case "ArrowDown":
        focus(rows[index + 1]);
        break;
      case "ArrowUp":
        focus(rows[index - 1]);
        break;
      case "Home":
        focus(rows[0]);
        break;
      case "End":
        focus(rows.at(-1));
        break;
      case "ArrowRight":
        if (row.kind === "folder") {
          if (!row.expanded) onCommand({ kind: "toggle", path: row.path });
          else if ((rows[index + 1]?.depth ?? 0) > row.depth) focus(rows[index + 1]);
        }
        break;
      case "ArrowLeft":
        if (row.kind === "folder" && row.expanded) onCommand({ kind: "toggle", path: row.path });
        else focus(rows.slice(0, index).findLast((parent) => parent.depth === row.depth - 1));
        break;
      case "Enter":
        onCommand(
          row.kind === "folder"
            ? { kind: "toggle", path: row.path }
            : {
                kind: "open",
                path: row.path,
                click: { ctrl: event.ctrlKey, alt: event.altKey, middle: false },
              },
        );
        break;
      case "F2":
        onCommand({ kind: "rename", path: row.path });
        break;
      default:
        return;
    }
    event.preventDefault();
    event.stopPropagation();
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FilesToolbar anyExpanded={props.anyExpanded} onCommand={onCommand} />
      {props.loaded && rows.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-8">
          <span className="text-empty text-secondary">No notes yet</span>
          <button type="button" className="text-empty text-accent" onClick={() => onCommand({ kind: "newNote" })}>
            New note
          </button>
        </div>
      ) : (
        <div
          role="tree"
          aria-label="Files"
          tabIndex={-1}
          className="min-h-0 flex-1 overflow-auto pb-2"
          onKeyDown={keyDown}
          onContextMenu={(event) => {
            if (event.target !== event.currentTarget) return;
            event.preventDefault();
            event.stopPropagation();
            onCommand({ kind: "menu", path: null, folder: true, x: event.clientX, y: event.clientY });
          }}
          onDragOver={(event) => {
            if (!event.dataTransfer.types.includes(DRAG_TYPE)) return;
            event.preventDefault();
            event.stopPropagation();
            event.dataTransfer.dropEffect = "move";
            setDropFolder(targetFolder(event.target));
          }}
          onDrop={(event) => {
            setDropFolder(null);
            if (!event.dataTransfer.types.includes(DRAG_TYPE)) return;
            event.preventDefault();
            event.stopPropagation();
            let paths: unknown;
            try {
              paths = JSON.parse(event.dataTransfer.getData(DRAG_TYPE));
            } catch {
              return;
            }
            if (Array.isArray(paths) && paths.every((path): path is string => typeof path === "string")) {
              onCommand({ kind: "drop", paths, folder: targetFolder(event.target) });
            }
          }}
          onDragLeave={(event) => {
            if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) {
              setDropFolder(null);
            }
          }}
        >
          {rows.map((row) => (
            // biome-ignore lint/a11y/useKeyWithClickEvents: The tree handles keyboard events for all rows.
            <div
              key={row.path}
              ref={(element) => {
                if (element) elements.current.set(row.path, element);
                else elements.current.delete(row.path);
              }}
              role="treeitem"
              aria-level={row.depth + 1}
              aria-expanded={row.kind === "folder" ? row.expanded : undefined}
              aria-selected={row.path === active}
              aria-label={row.name}
              data-path={row.path}
              title={row.path}
              draggable={row.path !== renaming}
              tabIndex={row.path === tabPath ? 0 : -1}
              className={`text-sidebar-row relative mx-2 flex h-7 min-w-0 items-center rounded-md pointer-coarse:h-11 ${
                row.path === active ? "bg-selection-sidebar" : "hover:bg-selection-inactive"
              } ${row.kind === "folder" && row.path === dropFolder ? "ring-1 ring-accent ring-inset" : ""}`}
              style={{ paddingLeft: 4 + 16 * row.depth }}
              onFocus={() => setFocused(row.path)}
              onClick={(event) => {
                focus(row);
                onCommand(
                  row.kind === "folder"
                    ? { kind: "toggle", path: row.path }
                    : {
                        kind: "open",
                        path: row.path,
                        click: paneClick(event),
                      },
                );
              }}
              onAuxClick={(event) => {
                if (event.button === 1 && row.kind === "file") {
                  onCommand({ kind: "open", path: row.path, click: paneClick(event) });
                }
              }}
              onContextMenu={(event) => {
                event.preventDefault();
                event.stopPropagation();
                onCommand({
                  kind: "menu",
                  path: row.path,
                  folder: row.kind === "folder",
                  x: event.clientX,
                  y: event.clientY,
                });
              }}
              onDragStart={(event) => {
                event.dataTransfer.setData(DRAG_TYPE, JSON.stringify([row.path]));
                event.dataTransfer.setData("text/plain", row.path);
                event.dataTransfer.effectAllowed = "all";
              }}
            >
              {props.indentGuides &&
                [...Array(row.depth).keys()].map((level) => (
                  <span
                    key={level}
                    data-guide
                    aria-hidden="true"
                    className="pointer-events-none absolute inset-y-0 w-px bg-separator"
                    style={{ left: 4 + 16 * level + 8 }}
                  />
                ))}
              <span className="w-4 shrink-0 text-secondary">
                {row.kind === "folder" && <ChevronRight size={14} className={row.expanded ? "rotate-90" : ""} />}
              </span>
              {row.path === renaming ? (
                <RenameField row={row} onCommand={onCommand} />
              ) : (
                <span className="truncate">{row.name}</span>
              )}
              {row.kind === "file" && row.type !== "note" && row.extension && (
                <span className="text-keycap ml-1.5 shrink-0 rounded bg-badge px-1 text-badge-text uppercase">
                  {row.extension}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function FilesToolbar({ anyExpanded, onCommand }: Pick<FilesPaneProps, "anyExpanded" | "onCommand">) {
  return (
    <div className={paneToolbar}>
      <IconButton
        size={24}
        label="New note"
        keys="Ctrl + N"
        icon={SquarePen}
        onClick={() => onCommand({ kind: "newNote" })}
      />
      <IconButton size={24} label="New folder" icon={FolderPlus} onClick={() => onCommand({ kind: "newFolder" })} />
      <IconButton
        size={24}
        label="Change sort order"
        icon={ArrowUpNarrowWide}
        onClick={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          onCommand({ kind: "sortMenu", x: rect.left, y: rect.bottom });
        }}
      />
      <IconButton
        size={24}
        label={anyExpanded ? "Collapse all" : "Expand all"}
        icon={anyExpanded ? ChevronsDownUp : ChevronsUpDown}
        onClick={() => onCommand({ kind: anyExpanded ? "collapseAll" : "expandAll" })}
      />
    </div>
  );
}

function RenameField({ row, onCommand }: { row: TreeRow; onCommand: FilesPaneProps["onCommand"] }) {
  const [name, setName] = useState(row.name);
  const input = useRef<HTMLInputElement>(null);
  const finished = useRef(false);
  const invalid = name.includes("/");
  useEffect(() => {
    input.current?.focus();
    input.current?.select();
  }, []);

  function finish(command: FilesCommand) {
    if (finished.current) return;
    finished.current = true;
    onCommand(command);
  }

  return (
    <input
      ref={input}
      aria-label="Name"
      aria-invalid={invalid}
      spellCheck={false}
      value={name}
      className={`h-6 min-w-0 flex-1 rounded border bg-window ${invalid ? "border-flag-1" : "border-accent"}`}
      onChange={(event) => setName(event.target.value)}
      onClick={(event) => event.stopPropagation()}
      onAuxClick={(event) => event.stopPropagation()}
      onKeyUp={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        event.stopPropagation();
        if (event.key === "Enter") {
          event.preventDefault();
          if (!invalid) finish({ kind: "renamed", path: row.path, name: name.trim() });
        } else if (event.key === "Escape") {
          event.preventDefault();
          finish({ kind: "cancelRename" });
        }
      }}
      onBlur={() =>
        finish(
          !invalid && name.trim() ? { kind: "renamed", path: row.path, name: name.trim() } : { kind: "cancelRename" },
        )
      }
    />
  );
}
