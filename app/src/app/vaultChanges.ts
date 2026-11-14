// When the views of a note's links read notesd again (docs/design/app.md,
// Panes): each save of a note changes the files store, but few saves
// change a link. So the Backlinks and Outgoing links panes and the status
// bar's backlinks read again at once when the set of paths changes (a
// file added, moved or removed, which can change where a link leads), and
// otherwise once the vault's files stop changing: a person who types with
// short pauses saves every second or so, and costs 1 read, not 1 a save.
import { useEffect, useRef, useState } from "react";

import { useFiles } from "../data/files";
import { SAVE_DELAY } from "../editor/document";

/** LINKS_DELAY is how long the vault's files must stay unchanged before the links are read again, in ms: longer than an editor's SAVE_DELAY, so that saves while typing come closer together than it. */
export const LINKS_DELAY = 2 * SAVE_DELAY;

/**
 * useLinksVersion counts the vault's changes that may change links: it
 * goes up at once when the set of paths changes, and delay ms after the
 * last change to a file's content otherwise. A view of links reads again
 * when it goes up.
 */
export function useLinksVersion(delay = LINKS_DELAY): number {
  const files = useFiles((s) => s.files);
  const pathsVersion = useFiles((s) => s.pathsVersion);
  const [version, setVersion] = useState(0);
  const seen = useRef({ files, pathsVersion });
  useEffect(() => {
    const was = seen.current;
    if (was.files === files && was.pathsVersion === pathsVersion) return;
    seen.current = { files, pathsVersion };
    if (was.pathsVersion !== pathsVersion) {
      setVersion((v) => v + 1);
      return;
    }
    const timer = setTimeout(() => setVersion((v) => v + 1), delay);
    return () => clearTimeout(timer);
  }, [files, pathsVersion, delay]);
  return version;
}
