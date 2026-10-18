// The vault manager's create form validates the name and delegates folder
// selection and creation to its container (docs/specs/ui.md#vault-manager).
import { ArrowLeft } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { vaultNameError } from "../../lib/vaultName";

export interface CreateVaultProps {
  /** The folder the vault goes in, once one is chosen. */
  location: string | null;
  /** The system has a folder picker: Browse opens it. Else the folder is typed. */
  canPick: boolean;
  /** Browse: the container opens the picker and gives back location. */
  onBrowse: () => void;
  /** Where there is no picker, the folder as typed. */
  onLocation: (path: string) => void;
  /** Create, with the name trimmed. */
  onCreate: (name: string) => void;
  onBack: () => void;
}

/** CreateVault draws the create form. */
export function CreateVault(props: CreateVaultProps) {
  const { location, canPick, onBrowse, onLocation, onCreate, onBack } = props;
  const [name, setName] = useState("");
  const [edited, setEdited] = useState(false);
  const nameId = useId();
  const errorId = useId();
  const nameInput = useRef<HTMLInputElement>(null);
  const error = vaultNameError(name);
  const showError = edited && error !== null;
  const canCreate = error === null && !!location?.trim();

  useEffect(() => {
    nameInput.current?.focus();
  }, []);

  return (
    <section aria-label="Create new vault" className="space-y-4 text-sidebar-row">
      <div className="flex items-center gap-2">
        <button
          type="button"
          aria-label="Back"
          title="Back"
          className="flex h-7 w-7 items-center justify-center rounded-md hover:bg-selection-inactive"
          onClick={onBack}
        >
          <ArrowLeft size={16} aria-hidden="true" />
        </button>
        <h2 className="font-semibold text-empty">Create new vault</h2>
      </div>
      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (canCreate) onCreate(name.trim());
        }}
      >
        <div className="space-y-1">
          <label htmlFor={nameId} className="block">
            Vault name
          </label>
          <input
            ref={nameInput}
            id={nameId}
            type="text"
            value={name}
            aria-invalid={showError || undefined}
            aria-describedby={showError ? errorId : undefined}
            className="h-7 w-60 max-w-full rounded-md border border-separator bg-window px-2"
            onChange={(event) => {
              setName(event.target.value);
              setEdited(true);
            }}
          />
          {showError && (
            <p id={errorId} className="text-flag-1 text-xs leading-4">
              {error}
            </p>
          )}
        </div>
        <div className="space-y-1">
          <div>Location</div>
          {canPick ? (
            <div className="flex min-w-0 items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-secondary text-xs leading-4">
                {location ?? "No folder chosen"}
              </span>
              <button
                type="button"
                className="h-7 shrink-0 rounded-md border border-separator px-3 hover:bg-selection-inactive"
                onClick={onBrowse}
              >
                Browse
              </button>
            </div>
          ) : (
            <input
              type="text"
              aria-label="Location"
              value={location ?? ""}
              className="h-7 w-60 max-w-full rounded-md border border-separator bg-window px-2"
              onChange={(event) => onLocation(event.target.value)}
            />
          )}
        </div>
        <button
          type="submit"
          disabled={!canCreate}
          className="h-7 rounded-md bg-accent px-3 font-medium text-accent-contrast disabled:opacity-50"
        >
          Create
        </button>
      </form>
    </section>
  );
}
