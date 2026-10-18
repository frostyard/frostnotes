// The Backlinks pane (docs/specs/ui.md, Right sidebar panes). Task T-0057
// replaces this stub.
import type { SnippetPart } from "../../rpc/gen/api";
import type { PaneClick } from "../panes/common";

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
export function BacklinksPane(_props: BacklinksPaneProps) {
  return null;
}
