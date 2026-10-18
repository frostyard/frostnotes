// The vault manager's create form (docs/specs/ui.md#vault-manager). Task
// T-0083 replaces this stub.

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
export function CreateVault(_props: CreateVaultProps) {
  return null;
}
