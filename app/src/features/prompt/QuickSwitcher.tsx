// The quick switcher (docs/specs/ui.md, Prompts): Ctrl+O's prompt over
// the vault's notes, with highlighted names and aliases and open/create actions.
import { Forward } from "lucide-react";
import { useMemo, useState } from "react";

import { type SwitcherNote, type SwitcherResult, stripMd, switcherResults } from "../../lib/switcher";
import { highlight, Prompt, type PromptMods } from "./Prompt";

/** SwitcherWhere is where a note opens: in the current tab, a new tab, or to the right. */
export type SwitcherWhere = "current" | "tab" | "right";

export interface QuickSwitcherProps {
  notes: readonly SwitcherNote[];
  /** The recently opened notes' paths, newest first. */
  recent: readonly string[];
  onOpen: (path: string, where: SwitcherWhere) => void;
  /** Create a note named name (as typed, without ".md"). */
  onCreate: (name: string, where: SwitcherWhere) => void;
  onClose: () => void;
}

/** whereFor maps the modifiers of Enter or a click to where the note opens. */
export function whereFor(mods: PromptMods): SwitcherWhere {
  return mods.ctrl ? (mods.alt ? "right" : "tab") : "current";
}

/** QuickSwitcher draws the quick switcher. */
export function QuickSwitcher({ notes, recent, onOpen, onCreate, onClose }: QuickSwitcherProps) {
  const [query, setQuery] = useState("");
  const items = useMemo(() => switcherResults(notes, query, recent), [notes, query, recent]);

  function onEnter(item: SwitcherResult | undefined, mods: PromptMods) {
    const where = whereFor(mods);
    const name = query.trim();
    if (mods.shift || !item) {
      if (!name) return;
      onClose();
      onCreate(name, where);
    } else {
      onClose();
      onOpen(item.path, where);
    }
  }

  return (
    <Prompt
      label="Quick switcher"
      placeholder="Find or create a note…"
      query={query}
      onQuery={setQuery}
      items={items}
      itemKey={(item) => (item.kind === "note" ? `note:${item.path}` : `alias:${item.path}:${item.alias}`)}
      renderItem={renderResult}
      onEnter={onEnter}
      onClose={onClose}
      instructions={[
        { keys: "↑↓", text: "to navigate" },
        { keys: "↵", text: "to open" },
        { keys: "ctrl ↵", text: "to open in new tab" },
        { keys: "ctrl alt ↵", text: "to open to the right" },
        { keys: "shift ↵", text: "to create" },
        { keys: "esc", text: "to dismiss" },
      ]}
      emptyText={query.trim() ? "No notes found." : "No recent files."}
    />
  );
}

function renderResult(item: SwitcherResult) {
  const path = stripMd(item.path);
  const from = path.lastIndexOf("/") + 1;
  const name = path.slice(from);
  const ranges = item.match?.matches ?? [];
  if (item.kind === "alias") {
    return (
      <>
        <span className="truncate text-sidebar-row">{highlight(item.alias, ranges)}</span>
        <Forward size={12} aria-hidden="true" className="ml-2 shrink-0 text-secondary" />
        <span className="ml-2 truncate text-xs/4 text-secondary">{name}</span>
      </>
    );
  }
  const folder = path.slice(0, Math.max(0, from - 1));
  return (
    <>
      <span className="truncate text-sidebar-row">{highlight(name, ranges, from)}</span>
      {folder && <span className="ml-2 truncate text-xs/4 text-secondary">{highlight(folder, ranges, 0)}</span>}
    </>
  );
}
