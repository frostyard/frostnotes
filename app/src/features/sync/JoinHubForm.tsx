// The join form (docs/specs/ui.md#vault-manager, docs/specs/ui.md#settings):
// the hub's URL and vault name, this device's name and, for a new vault, its
// folder; then the code to approve on the hub, and the wait. Its container
// (app/JoinHub.tsx) runs the flow.
import { ArrowLeft } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { vaultNameError } from "../../lib/vaultName";

/** JoinPhase is what the form shows: its fields, the code to approve, or the wait. */
export type JoinPhase = "form" | "approve" | "joining";

/** JoinMode is what the form is for: joining a vault, or pairing again with a new code. */
export type JoinMode = "join" | "pair";

/** JoinFields is what the person typed, trimmed. */
export interface JoinFields {
  /** The hub's URL. */
  url: string;
  /** The hub vault's name; "" in the pair-again mode. */
  name: string;
  /** This device's name; "" for notesd's default, the host name. */
  device: string;
}

/** JoinLocation is the folder for the new vault, as the create form has it. */
export interface JoinLocation {
  /** The folder the new vault goes in, once one is chosen. */
  path: string | null;
  /** The system has a folder picker: Browse opens it. Else the folder is typed. */
  canPick: boolean;
  /** Browse: the container opens the picker and gives back the folder. */
  onBrowse: () => void;
  /** Where there is no picker, the folder as typed. */
  onLocation: (path: string) => void;
}

export interface JoinHubFormProps {
  /** "join" asks for the hub vault's name and makes a vault; "pair" pairs again with a new code. */
  mode: JoinMode;
  phase: JoinPhase;
  /** The hub URL the fields start with. */
  url?: string;
  /** The code to approve on the hub, in the approve phase. */
  code?: string | null;
  /** Why the last attempt failed, in the person's words. */
  error?: string | null;
  /** The folder for a new vault, where the form makes one (the vault manager's). */
  location?: JoinLocation;
  /** Join, with the fields trimmed. */
  onJoin: (fields: JoinFields) => void;
  /** Back: the fields go and the panel closes. */
  onBack: () => void;
  /** Cancel, while the device waits for the approval. */
  onCancel: () => void;
}

/**
 * hubNameProblem says why a hub vault's name is not one, or null: a new
 * vault needs a vault's name, a vault that exists a name at all.
 */
function hubNameProblem(name: string, makesVault: boolean): string | null {
  if (makesVault) return vaultNameError(name);
  return name.trim() === "" ? "A vault needs a name." : null;
}

/** JoinHubForm draws the join form. */
export function JoinHubForm(props: JoinHubFormProps) {
  const { mode, phase, code, error, location, onJoin, onBack, onCancel } = props;
  const pair = mode === "pair";
  const [url, setURL] = useState(props.url ?? "");
  const [name, setName] = useState("");
  const [device, setDevice] = useState("");
  const [edited, setEdited] = useState(false);
  const urlId = useId();
  const nameId = useId();
  const nameErrorId = useId();
  const deviceId = useId();
  const urlInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    urlInput.current?.focus();
  }, []);

  // A new vault's name is a vault's name: the folder takes it. Where the
  // vault exists already (the settings), the hub hears the name only.
  const makesVault = location !== undefined;
  const nameProblem = pair ? null : hubNameProblem(name, makesVault);
  const showNameProblem = edited && nameProblem !== null;
  const located = location === undefined || !!location.path?.trim();
  // Join needs the hub's URL, a name and, for a new vault, a folder.
  const canJoin = url.trim() !== "" && nameProblem === null && located;

  return (
    <section aria-label={pair ? "Pair again" : "Join a vault on a hub"} className="space-y-4 text-sidebar-row">
      <div className="flex items-center gap-2">
        {/* Back stops the flow: only the fields have one to go back to. */}
        {phase === "form" && (
          <button
            type="button"
            aria-label="Back"
            title="Back"
            className="flex h-7 w-7 items-center justify-center rounded-md hover:bg-selection-inactive"
            onClick={onBack}
          >
            <ArrowLeft size={16} aria-hidden="true" />
          </button>
        )}
        <h2 className="font-semibold text-empty">{pair ? "Pair again" : "Join a vault on a hub"}</h2>
      </div>
      {phase === "form" && (
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (canJoin) onJoin({ url: url.trim(), name: pair ? "" : name.trim(), device: device.trim() });
          }}
        >
          <div className="space-y-1">
            <label htmlFor={urlId} className="block">
              Hub URL
            </label>
            <input
              ref={urlInput}
              id={urlId}
              type="text"
              value={url}
              spellCheck={false}
              placeholder="https://lodge.example.ts.net/device/notes"
              className="h-7 w-[320px] max-w-full rounded-md border border-separator bg-window px-2"
              onChange={(event) => setURL(event.target.value)}
            />
          </div>
          {!pair && (
            <div className="space-y-1">
              <label htmlFor={nameId} className="block">
                Hub vault name
              </label>
              <input
                id={nameId}
                type="text"
                value={name}
                spellCheck={false}
                aria-invalid={showNameProblem || undefined}
                aria-describedby={showNameProblem ? nameErrorId : undefined}
                className="h-7 w-60 max-w-full rounded-md border border-separator bg-window px-2"
                onChange={(event) => {
                  setName(event.target.value);
                  setEdited(true);
                }}
              />
              {showNameProblem && (
                <p id={nameErrorId} className="text-flag-1 text-xs leading-4">
                  {nameProblem}
                </p>
              )}
            </div>
          )}
          <div className="space-y-1">
            <label htmlFor={deviceId} className="block">
              This device&apos;s name (optional)
            </label>
            <input
              id={deviceId}
              type="text"
              value={device}
              spellCheck={false}
              placeholder="This computer's name"
              className="h-7 w-60 max-w-full rounded-md border border-separator bg-window px-2"
              onChange={(event) => setDevice(event.target.value)}
            />
          </div>
          {location !== undefined && (
            <div className="space-y-1">
              <div>Location</div>
              {location.canPick ? (
                <div className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-secondary text-xs leading-4">
                    {location.path ?? "No folder chosen"}
                  </span>
                  <button
                    type="button"
                    className="h-7 shrink-0 rounded-md border border-separator px-3 hover:bg-selection-inactive"
                    onClick={location.onBrowse}
                  >
                    Browse
                  </button>
                </div>
              ) : (
                <input
                  type="text"
                  aria-label="Location"
                  value={location.path ?? ""}
                  spellCheck={false}
                  className="h-7 w-60 max-w-full rounded-md border border-separator bg-window px-2"
                  onChange={(event) => location.onLocation(event.target.value)}
                />
              )}
            </div>
          )}
          <button
            type="submit"
            disabled={!canJoin}
            className="h-7 rounded-md bg-accent px-3 font-medium text-accent-contrast disabled:opacity-50"
          >
            Join
          </button>
        </form>
      )}
      {phase === "approve" && (
        <div className="space-y-4">
          <p className="select-text font-mono text-xl leading-7">{code}</p>
          <p className="text-xs leading-4 text-secondary">
            On the hub&apos;s machine, approve this device:{" "}
            <code className="select-text font-mono">notesctl hub approve {code}</code>. The code expires in 10 minutes.
          </p>
          <button
            type="button"
            className="h-7 rounded-md border border-separator px-3 hover:bg-selection-inactive"
            onClick={onCancel}
          >
            Cancel
          </button>
        </div>
      )}
      {phase === "joining" && (
        <p role="status" className="text-sm leading-5 text-secondary">
          Joining…
        </p>
      )}
      {error !== null && error !== undefined && <p className="text-flag-1 text-xs leading-4">{error}</p>}
    </section>
  );
}
