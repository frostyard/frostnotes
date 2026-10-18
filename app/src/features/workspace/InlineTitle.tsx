// The inline title (docs/specs/ui.md, Editor and reading view). Task
// T-0078 replaces this stub.

/** FORBIDDEN are the characters a note's name may not hold. */
export const FORBIDDEN = /[\\/:*?"<>|#^[\]]/;

export interface InlineTitleProps {
  /** The file's name without its extension. */
  title: string;
  /** Rename the file to a new, valid name. */
  onRename: (title: string) => void;
  /** The title was left with Enter or Escape: the editor may take focus. */
  onDone?: () => void;
}

/** InlineTitle draws a note's editable title. */
export function InlineTitle({ title }: InlineTitleProps) {
  return <h1>{title}</h1>;
}
