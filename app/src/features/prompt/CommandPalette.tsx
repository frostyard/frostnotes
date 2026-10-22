// The command palette (docs/specs/ui.md, Prompts): Ctrl+P's prompt over
// the commands that apply now, with fuzzy highlights and shortcut caps.

import { highlight, Prompt } from "@frostyard/lodge-ui/react";
import { useMemo, useState } from "react";
import { type PaletteCommand, paletteResults } from "../../lib/palette";

export interface CommandPaletteProps {
  /** The commands that apply now. */
  commands: readonly PaletteCommand[];
  /** The IDs of the commands run, newest first. */
  recent: readonly string[];
  onRun: (id: string) => void;
  onClose: () => void;
}

/** CommandPalette draws the command palette. */
export function CommandPalette({ commands, recent, onRun, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const items = useMemo(() => paletteResults(commands, query, recent), [commands, query, recent]);

  return (
    <Prompt
      label="Command palette"
      placeholder="Select a command…"
      query={query}
      onQuery={setQuery}
      items={items}
      itemKey={({ command }) => command.id}
      renderItem={({ command, match }) => (
        <>
          <span className="min-w-0 flex-1 truncate text-sidebar-row">
            {highlight(command.name, match?.matches ?? [])}
          </span>
          {command.keys.length > 0 && (
            <span className="ml-2 flex shrink-0 gap-1">
              {command.keys.map((keys) => (
                <kbd key={keys} className="rounded-[4px] bg-badge px-1.5 py-0.5 font-sans text-badge-text text-keycap">
                  {keys}
                </kbd>
              ))}
            </span>
          )}
        </>
      )}
      onEnter={(item) => {
        if (!item) return;
        onClose();
        onRun(item.command.id);
      }}
      onClose={onClose}
      instructions={[
        { keys: "↑↓", text: "to navigate" },
        { keys: "↵", text: "to use" },
        { keys: "esc", text: "to dismiss" },
      ]}
      emptyText="No commands found."
    />
  );
}
