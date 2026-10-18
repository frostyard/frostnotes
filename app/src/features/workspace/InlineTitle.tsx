// The inline title (docs/specs/ui.md, Editor and reading view) validates
// edits on blur and leaves renaming the file to its container.
import { useEffect, useRef, useState } from "react";

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
export function InlineTitle({ title, onRename, onDone }: InlineTitleProps) {
  const [text, setText] = useState(title);
  const [invalid, setInvalid] = useState(false);
  const cancelled = useRef(false);

  useEffect(() => {
    setText(title);
    setInvalid(false);
  }, [title]);

  function commit() {
    if (cancelled.current) {
      cancelled.current = false;
      return;
    }
    const name = text.trim();
    if (!name || name === title) {
      setText(title);
    } else if (FORBIDDEN.test(name)) {
      setText(title);
      setInvalid(true);
    } else {
      setText(name);
      onRename(name);
    }
  }

  return (
    <>
      <input
        type="text"
        aria-label="Title"
        spellCheck={false}
        className="mb-[0.5em] block w-full border-0 bg-transparent text-[1.618em] font-bold leading-[1.2] text-primary"
        value={text}
        onChange={(event) => {
          setText(event.currentTarget.value);
          setInvalid(false);
        }}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key !== "Enter" && event.key !== "Escape") return;
          event.preventDefault();
          if (event.key === "Escape") {
            cancelled.current = true;
            setText(title);
            setInvalid(false);
          }
          event.currentTarget.blur();
          onDone?.();
        }}
      />
      {invalid && <p role="alert">{'A file name cannot contain \\ / : * ? " < > | # ^ [ ].'}</p>}
    </>
  );
}
