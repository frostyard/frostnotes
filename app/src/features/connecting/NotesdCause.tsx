// Why notesd is not running, under the window's "Reconnecting to notesd…"
// (docs/specs/ui.md#connecting): in the Flatpak, the launcher's note that
// the Flatpak portal did not start notesd, with the checks that fix it, or
// else notesd.log's last line; both name the log.
import type { NotesdLog } from "../../data/notesdLog";

const SHOW = "flatpak override --user --show org.frostyard.Frostnotes";
const RESET = "flatpak override --user --reset org.frostyard.Frostnotes";

function Cmd({ text }: { text: string }) {
  return <code className="rounded bg-code px-1 font-mono select-text">{text}</code>;
}

function PortalFix() {
  return (
    <>
      <p>notesd did not start, because the Flatpak portal did not start it. To fix it:</p>
      <ol className="list-decimal pl-5">
        <li>
          Run <Cmd text={SHOW} />: it must not list a blocked <Cmd text="org.freedesktop.portal.Flatpak" />.
        </li>
        <li>
          Remove the overrides with <Cmd text={RESET} />, if it does.
        </li>
        <li>
          Check that <Cmd text="xdg-desktop-portal" /> runs.
        </li>
      </ol>
    </>
  );
}

function LastLine({ line }: { line: string | null }) {
  if (line === null) return null;
  return (
    <p>
      The last line of notesd's log: <span className="select-text">{line}</span>
    </p>
  );
}

/** NotesdCause draws the cause and the fix that notesd.log tells. */
export function NotesdCause({ log }: { log: NotesdLog }) {
  return (
    <section aria-label="Why notesd is not running" className="flex max-w-[560px] flex-col gap-2 text-left text-xs">
      {log.portalRefused ? <PortalFix /> : <LastLine line={log.lastLine} />}
      <p>
        The log: <span className="font-mono select-text">{log.path}</span>
      </p>
    </section>
  );
}
