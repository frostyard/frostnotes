// The Tasks pane's container reads note.tasks and opens a task at its line.
import { useEffect, useState } from "react";

import { useFiles } from "../../data/files";
import { notify } from "../../data/overlays";
import { useClient } from "../../data/session";
import { useNotes } from "../../data/stores";
import { useDocuments } from "../../editor/registry";
import { type TaskGroup, TasksPane } from "../../features/tasks/TasksPane";
import type { TaskEntry } from "../../rpc/gen/api";
import { openAtLine, openFile } from "./open";

const titleOf = (p: string) => p.slice(p.lastIndexOf("/") + 1).replace(/\.md$/i, "");

/** taskGroups groups tasks by path in the order note.tasks gives. */
export function taskGroups(tasks: readonly TaskEntry[]): TaskGroup[] {
  const groups = new Map<string, TaskGroup>();
  for (const task of tasks) {
    let group = groups.get(task.path);
    if (!group) {
      group = { path: task.path, title: task.title || titleOf(task.path), tasks: [] };
      groups.set(task.path, group);
    }
    (group.tasks as { line: number; text: string }[]).push({ line: task.line, text: task.text });
  }
  return [...groups.values()];
}

/** TasksContainer reads the vault's open tasks and reloads them when files change. */
export function TasksContainer() {
  const client = useClient();
  const docs = useDocuments();
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  const [state, setState] = useState<{ vaultId: number; groups: TaskGroup[] } | null>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger: tasks may change when files change
  useEffect(() => {
    if (vaultId === null) return;
    let stale = false;
    client.note
      .tasks({ vaultId })
      .then((tasks) => !stale && setState({ vaultId, groups: taskGroups(tasks) }))
      .catch((err: unknown) => {
        if (!stale) {
          console.error("tasks:", err);
          setState({ vaultId, groups: [] });
        }
      });
    return () => {
      stale = true;
    };
  }, [client, vaultId, files]);

  const groups = state !== null && state.vaultId === vaultId ? state.groups : null;
  return (
    <TasksPane
      groups={groups}
      onOpen={(path, line, click) => {
        if (line === null || vaultId === null) return openFile(path, click);
        openAtLine(client, docs, vaultId, path, line, click).catch((err: unknown) =>
          notify(err instanceof Error ? err.message : String(err)),
        );
      }}
    />
  );
}
