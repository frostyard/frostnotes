// The template chooser (docs/specs/ui.md, Prompts; Insert template): a
// searchable prompt over the template folder's notes, shown by name.
import { useMemo, useState } from "react";

import { type TemplateResult, templateResults } from "../../lib/templates";
import { highlight, Prompt } from "./Prompt";

export interface TemplateChooserProps {
  /** The template folder's notes, by path. */
  templates: readonly string[];
  /** The template chosen, by path. */
  onChoose: (path: string) => void;
  onClose: () => void;
}

/** TemplateChooser draws the template chooser. */
export function TemplateChooser({ templates, onChoose, onClose }: TemplateChooserProps) {
  const [query, setQuery] = useState("");
  const items = useMemo(() => templateResults(templates, query), [templates, query]);

  function onEnter(item: TemplateResult | undefined) {
    if (!item) return;
    onClose();
    onChoose(item.path);
  }

  return (
    <Prompt
      label="Insert template"
      placeholder="Choose a template…"
      query={query}
      onQuery={setQuery}
      items={items}
      itemKey={(item) => `template:${item.path}`}
      renderItem={({ name, match }) => (
        <span className="truncate text-sidebar-row">{highlight(name, match?.matches ?? [])}</span>
      )}
      onEnter={onEnter}
      onClose={onClose}
      instructions={[
        { keys: "↑↓", text: "to navigate" },
        { keys: "↵", text: "to insert" },
        { keys: "esc", text: "to dismiss" },
      ]}
      emptyText="No templates found."
    />
  );
}
