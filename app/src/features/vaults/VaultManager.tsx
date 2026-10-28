// The vault manager (docs/specs/ui.md#vault-manager): registered vaults,
// folder opening and creation actions in a window or a modal frame.

import { Menu, Sheet } from "@frostyard/lodge-ui/react";
import { Ellipsis, X } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

/** SyncAvailability is whether this notesd syncs: unknown while the check runs. */
export type SyncAvailability = "unknown" | "yes" | "no";

/** JOIN_WORDS is the join row's words, by whether this notesd syncs. */
const JOIN_WORDS: Record<SyncAvailability, string> = {
  unknown: "Checking whether this notesd syncs…",
  yes: "Sync a new vault with a vault on a hub. To sync a vault you have, open it, then Settings, Sync.",
  no: "This notesd does not sync.",
};

/** VaultEntry is a vault the manager lists. */
export interface VaultEntry {
  id: number;
  name: string;
  path: string;
}

export interface VaultManagerProps {
  /** The vaults, in the order shown (by name). */
  vaults: VaultEntry[];
  /** The window's vault, if any. */
  current: number | null;
  /** "window" fills its parent, without Close; "dialog" floats over the window. */
  mode: "window" | "dialog";
  /** The system has a folder picker: Open opens it. Else a path is typed. */
  canPick: boolean;
  /** When given, shown in the actions' place: the create form. */
  panel?: ReactNode;
  /**
   * The caller may add and remove vaults (true by default). A browser
   * inside lodge may only open them: it gets no actions and no options.
   */
  manage?: boolean;
  onOpen: (id: number) => void;
  onRemove: (vault: VaultEntry) => void;
  onCreate: () => void;
  /** Join a vault on a hub: shows the join panel in the actions' place. */
  onJoin: () => void;
  /** Whether this notesd syncs: the Join button shows for "yes" alone. */
  syncAvailable: SyncAvailability;
  /** Open folder as vault: no path with a picker, else the typed path, trimmed. */
  onOpenFolder: (path?: string) => void;
  onClose: () => void;
}

/** VaultManager draws the vault manager. */
export function VaultManager(props: VaultManagerProps) {
  const { mode, panel, onClose } = props;
  const dialog = useRef<HTMLDivElement>(null);
  const modal = mode === "dialog";

  useEffect(() => {
    if (!modal) return;
    const before = document.activeElement;
    dialog.current?.focus();
    return () => {
      if (before instanceof HTMLElement && before.isConnected) before.focus();
    };
  }, [modal]);

  // The manager in a page and the manager in a dialog hold the same parts.
  const parts = (
    <>
      <VaultList {...props} />
      <div className="min-w-0 flex-1 overflow-y-auto p-8">
        {panel ?? (props.manage === false ? <NoActions /> : <VaultActions {...props} />)}
      </div>
      {modal && (
        <button
          type="button"
          aria-label="Close"
          title="Close"
          className="absolute top-2 right-2 flex size-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive"
          onClick={onClose}
        >
          <X size={16} aria-hidden="true" />
        </button>
      )}
    </>
  );

  if (modal) {
    return (
      <Sheet
        ariaLabel="Manage vaults"
        width={720}
        className="relative flex h-[480px] max-h-[85vh] max-w-[90vw] overflow-hidden p-0!"
        showClose={false}
        portal
        onClose={onClose}
      >
        {/* The package's frame pads itself for a form; this layout fills it. */}
        <div className="absolute inset-0 flex min-h-0 min-w-0">{parts}</div>
      </Sheet>
    );
  }
  return (
    <div
      ref={dialog}
      role="dialog"
      aria-label="Manage vaults"
      tabIndex={-1}
      className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden bg-window"
    >
      {parts}
    </div>
  );
}

function NoActions() {
  return (
    <p className="text-sm leading-5 text-secondary">
      Vaults are added and removed on the machine that runs notesd, with notesctl vault.
    </p>
  );
}

function VaultList({ vaults, current, manage, onOpen, onRemove }: VaultManagerProps) {
  const [menu, setMenu] = useState<{ vault: VaultEntry; x: number; y: number } | null>(null);
  const options = manage !== false;

  return (
    <div className="flex w-[280px] shrink-0 flex-col overflow-y-auto bg-sidebar p-2">
      {vaults.length === 0 ? (
        <p className="flex flex-1 items-center justify-center text-sidebar-row text-secondary">No vaults yet</p>
      ) : (
        <ul aria-label="Vaults">
          {vaults.map((vault) => (
            <li
              key={vault.id}
              className={`group relative rounded-md ${vault.id === current ? "bg-selection-sidebar" : "hover:bg-selection-inactive"}`}
              onContextMenu={(event) => {
                event.preventDefault();
                if (options) setMenu({ vault, x: event.clientX, y: event.clientY });
              }}
            >
              <button
                type="button"
                aria-current={vault.id === current ? true : undefined}
                className={`block h-[52px] w-full min-w-0 rounded-md pr-10 pl-3 text-left ${vault.id === current ? "bg-selection-sidebar" : ""}`}
                onClick={() => onOpen(vault.id)}
              >
                <span className="block truncate text-sidebar-row font-semibold">{vault.name}</span>
                <span className="block truncate text-xs leading-4 text-secondary">{vault.path}</span>
              </button>
              {options && (
                <button
                  type="button"
                  aria-label={`Options for ${vault.name}`}
                  title={`Options for ${vault.name}`}
                  aria-haspopup="menu"
                  aria-expanded={menu?.vault.id === vault.id}
                  className="absolute top-3.5 right-2 flex size-6 items-center justify-center rounded text-secondary opacity-0 group-hover:opacity-100 hover:bg-selection-inactive focus:opacity-100"
                  onClick={(event) => {
                    const rect = event.currentTarget.getBoundingClientRect();
                    setMenu({ vault, x: rect.left, y: rect.bottom });
                  }}
                >
                  <Ellipsis size={16} aria-hidden="true" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {menu && (
        <Menu
          label={`Options for ${menu.vault.name}`}
          items={[{ kind: "item", id: "remove", label: "Remove from list" }]}
          x={menu.x}
          y={menu.y}
          onSelect={() => onRemove(menu.vault)}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}

function VaultActions({ canPick, onCreate, onJoin, syncAvailable, onOpenFolder }: VaultManagerProps) {
  const [path, setPath] = useState("");

  function openFolder() {
    if (canPick) onOpenFolder();
    else if (path.trim()) onOpenFolder(path.trim());
  }

  return (
    <>
      <h2 className="mb-6 text-xl leading-7 font-semibold">Frostnotes</h2>
      <VaultAction title="Create new vault" description="Make a new vault in a folder you choose.">
        <button type="button" className="h-7 rounded-md bg-accent px-3 text-accent-contrast" onClick={onCreate}>
          Create
        </button>
      </VaultAction>
      <VaultAction title="Open folder as vault" description="Choose a folder of Markdown files.">
        {!canPick && (
          <input
            type="text"
            aria-label="Folder path"
            value={path}
            spellCheck={false}
            className="h-7 w-[240px] max-w-full rounded-md border border-separator bg-window px-2 focus:border-accent"
            onChange={(event) => setPath(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                openFolder();
              }
            }}
          />
        )}
        <button
          type="button"
          className="h-7 rounded-md border border-separator px-3 hover:bg-selection-inactive"
          onClick={openFolder}
        >
          Open
        </button>
      </VaultAction>
      <VaultAction title="Join a vault on a hub" description={JOIN_WORDS[syncAvailable]}>
        {syncAvailable === "yes" && (
          <button
            type="button"
            className="h-7 rounded-md border border-separator px-3 hover:bg-selection-inactive"
            onClick={onJoin}
          >
            Join
          </button>
        )}
      </VaultAction>
    </>
  );
}

function VaultAction({
  title,
  description,
  children,
}: {
  title: string;
  description: ReactNode;
  children?: ReactNode;
}) {
  return (
    <section
      aria-label={title}
      className="flex flex-wrap items-center justify-between gap-4 border-b border-separator py-4 last:border-b-0"
    >
      <div className="min-w-0 flex-1 basis-[180px]">
        <h3 className="text-sidebar-row font-semibold">{title}</h3>
        <p className="text-xs leading-4 text-secondary">{description}</p>
      </div>
      {children && <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">{children}</div>}
    </section>
  );
}
