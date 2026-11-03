// The Tasks pane draws open tasks grouped by note (docs/specs/ui.md).
import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { type PaneClick, paneClick, paneEmpty } from "../panes/common";

export interface TaskGroup {
  path: string;
  title: string;
  tasks: readonly { line: number; text: string }[];
}

export interface TasksPaneProps {
  groups: readonly TaskGroup[] | null;
  onOpen: (path: string, line: number | null, click: PaneClick) => void;
}

/** TasksPane draws one collapsible row for each note with open tasks. */
export function TasksPane({ groups, onOpen }: TasksPaneProps) {
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => new Set());

  function toggle(path: string) {
    setFolded((previous) => {
      const next = new Set(previous);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  if (groups === null) return null;
  if (groups.length === 0) return <p className={paneEmpty}>No open tasks.</p>;

  return (
    <ul aria-label="Tasks">
      {groups.map((group) => {
        const open = !folded.has(group.path);
        return (
          <li key={group.path}>
            <div className="text-sidebar-row flex h-7 min-w-0 items-center gap-1.5 pl-4 pr-2">
              <button
                type="button"
                aria-label={`${open ? "Fold" : "Unfold"} ${group.title}`}
                aria-expanded={open}
                tabIndex={-1}
                onClick={() => toggle(group.path)}
                className="shrink-0"
              >
                <ChevronRight size={14} className={`text-secondary ${open ? "rotate-90" : ""}`} />
              </button>
              <button
                type="button"
                title={group.path}
                className="min-w-0 flex-1 truncate text-left"
                onClick={(e) => onOpen(group.path, null, paneClick(e))}
                onAuxClick={(e) => {
                  if (e.button === 1) onOpen(group.path, null, paneClick(e));
                }}
              >
                {group.title}
              </button>
              <span className="text-status text-secondary">{group.tasks.length}</span>
            </div>
            {open && (
              <ul>
                {group.tasks.map((task) => (
                  <li key={`${task.line}:${task.text}`}>
                    <button
                      type="button"
                      aria-label={task.text}
                      className="w-full rounded-md py-1 pl-9 pr-2 text-left text-xs leading-4 hover:bg-selection-inactive pointer-coarse:min-h-11"
                      onClick={(e) => onOpen(group.path, task.line, paneClick(e))}
                      onAuxClick={(e) => {
                        if (e.button === 1) onOpen(group.path, task.line, paneClick(e));
                      }}
                    >
                      {task.text}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </li>
        );
      })}
    </ul>
  );
}
