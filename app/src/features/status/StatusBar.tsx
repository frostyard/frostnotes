// The status bar (ADR-0011): the connection to notesd for now; sync state
// and word count join it later.
import type { Connection } from "../../data/stores";
import type { Hello } from "../../rpc/gen/api";

export interface StatusBarProps {
  connection: Connection;
  hello: Hello | null;
  vaultCount: number;
}

function connectionText({ connection, hello }: StatusBarProps): string {
  switch (connection.state) {
    case "ready":
      return hello ? `Connected to ${hello.server}` : "Connected";
    case "connecting":
      return "Connecting to notesd…";
    case "lost":
      return `Reconnecting to notesd… (${connection.reason})`;
  }
}

export function StatusBar(props: StatusBarProps) {
  return (
    <footer
      role="status"
      className="text-status flex h-6 shrink-0 items-center gap-4 border-t border-separator bg-toolbar px-3 text-secondary"
    >
      <span className="min-w-0 flex-1 truncate">{connectionText(props)}</span>
      {props.connection.state === "ready" && (
        <span>
          {props.vaultCount} {props.vaultCount === 1 ? "vault" : "vaults"}
        </span>
      )}
    </footer>
  );
}
