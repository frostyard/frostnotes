// The template chooser (docs/specs/ui.md, Prompts; Insert template). Task
// T-0079 replaces this stub.

export interface TemplateChooserProps {
  /** The template folder's notes, by path. */
  templates: readonly string[];
  /** The template chosen, by path. */
  onChoose: (path: string) => void;
  onClose: () => void;
}

/** TemplateChooser draws the template chooser. */
export function TemplateChooser(_props: TemplateChooserProps) {
  return null;
}
