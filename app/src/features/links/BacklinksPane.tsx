// The Backlinks pane groups linked and unlinked mentions by source note,
// with collapsible sections and highlighted snippets (docs/specs/ui.md).
import { ChevronRight } from "lucide-react";
import { Fragment, useState } from "react";
import type { SnippetPart } from "../../rpc/gen/api";
import { type PaneClick, paneClick, paneEmpty } from "../panes/common";

/** Mention is a place a note mentions the active one: its line (null for an unlinked mention) and text. */
export interface Mention {
  line: number | null;
  parts: readonly SnippetPart[];
}

/** MentionGroup is a note's mentions of the active one. */
export interface MentionGroup {
  path: string;
  title: string;
  mentions: readonly Mention[];
}

export interface BacklinksPaneProps {
  /** The linked mentions; null when no note is open. */
  linked: readonly MentionGroup[] | null;
  /** The unlinked mentions; null while they load. */
  unlinked: readonly MentionGroup[] | null;
  /** Open the source note, at the line when given. */
  onOpen: (path: string, line: number | null, click: PaneClick) => void;
}

/** BacklinksPane draws the Backlinks pane. */
export function BacklinksPane({ linked, unlinked, onOpen }: BacklinksPaneProps) {
  const [linkedOpen, setLinkedOpen] = useState(true);
  const [unlinkedOpen, setUnlinkedOpen] = useState(false);
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => new Set());

  function toggleGroup(path: string) {
    setFolded((previous) => {
      const next = new Set(previous);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  if (linked === null) return <p className={paneEmpty}>No note is open.</p>;

  return (
    <>
      <MentionSection
        title="Linked mentions"
        empty="No backlinks found."
        groups={linked}
        open={linkedOpen}
        onToggle={() => setLinkedOpen((open) => !open)}
        folded={folded}
        onToggleGroup={toggleGroup}
        onOpen={onOpen}
      />
      <MentionSection
        title="Unlinked mentions"
        empty="No unlinked mentions found."
        groups={unlinked}
        open={unlinkedOpen}
        onToggle={() => setUnlinkedOpen((open) => !open)}
        folded={folded}
        onToggleGroup={toggleGroup}
        onOpen={onOpen}
      />
    </>
  );
}

interface MentionSectionProps {
  title: string;
  empty: string;
  groups: readonly MentionGroup[] | null;
  open: boolean;
  onToggle: () => void;
  folded: ReadonlySet<string>;
  onToggleGroup: (path: string) => void;
  onOpen: BacklinksPaneProps["onOpen"];
}

function MentionSection({ title, empty, groups, open, onToggle, folded, onToggleGroup, onOpen }: MentionSectionProps) {
  return (
    <section aria-label={title}>
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="text-sidebar-row mx-2 flex h-7 w-[calc(100%-1rem)] items-center gap-1.5 rounded-md hover:bg-selection-inactive"
      >
        <ChevronRight size={14} className={`shrink-0 text-secondary ${open ? "rotate-90" : ""}`} />
        <span className="min-w-0 flex-1 text-left font-semibold">{title}</span>
        {groups !== null && <span className="text-status text-secondary">{groups.length}</span>}
      </button>
      {open &&
        groups !== null &&
        (groups.length === 0 ? (
          <p className={paneEmpty}>{empty}</p>
        ) : (
          <ul>
            {groups.map((group) => (
              <MentionGroupRow
                key={group.path}
                group={group}
                open={!folded.has(group.path)}
                onToggle={() => onToggleGroup(group.path)}
                onOpen={onOpen}
              />
            ))}
          </ul>
        ))}
    </section>
  );
}

interface MentionGroupRowProps {
  group: MentionGroup;
  open: boolean;
  onToggle: () => void;
  onOpen: BacklinksPaneProps["onOpen"];
}

function MentionGroupRow({ group, open, onToggle, onOpen }: MentionGroupRowProps) {
  return (
    <li>
      <div className="text-sidebar-row flex h-7 min-w-0 items-center gap-1.5 pl-4 pr-2">
        <button
          type="button"
          aria-label={`${open ? "Fold" : "Unfold"} ${group.title}`}
          aria-expanded={open}
          tabIndex={-1}
          onClick={onToggle}
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
        <span className="text-status text-secondary">{group.mentions.length}</span>
      </div>
      {open && (
        <ul>
          {group.mentions.map((mention, index) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: Mentions are known by their place in the group.
            <li key={index}>
              <button
                type="button"
                aria-label={mention.parts.map((part) => part.text).join("")}
                className="w-full rounded-md py-1 pl-9 pr-2 text-left text-xs leading-4 hover:bg-selection-inactive"
                onClick={(e) => onOpen(group.path, mention.line, paneClick(e))}
                onAuxClick={(e) => {
                  if (e.button === 1) onOpen(group.path, mention.line, paneClick(e));
                }}
              >
                {mention.parts.map((part, partIndex) => (
                  // biome-ignore lint/suspicious/noArrayIndexKey: Snippet parts are known by their place in the mention.
                  <Fragment key={partIndex}>
                    {part.match ? (
                      <mark className="bg-highlight text-inherit">{part.text}</mark>
                    ) : (
                      <span>{part.text}</span>
                    )}
                  </Fragment>
                ))}
              </button>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
