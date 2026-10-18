// The properties block (docs/specs/ui.md, Editor and reading view). Task
// T-0077 replaces this stub.
import type { PropertyType, PropertyValue } from "../../editor/properties";

/** PropertyRow is a row of the block: a key, its type and its value. */
export interface PropertyRow {
  key: string;
  type: PropertyType;
  value: PropertyValue;
}

export interface PropertiesBlockProps {
  rows: readonly PropertyRow[];
  /** Why the frontmatter is not properties; null when it is. */
  error: string | null;
  folded: boolean;
  /** Each time it changes (but not at first), the block opens its new row. */
  addRequest?: number;
  onFold: (folded: boolean) => void;
  /** Set a key, new or not, to a value. */
  onChange: (key: string, value: PropertyValue) => void;
  onRename: (from: string, to: string) => void;
  onRemove: (key: string) => void;
}

/** items is a list value's items as text; any other value is one item. */
export function items(_value: PropertyValue): string[] {
  return [];
}

/** PropertiesBlock draws a note's properties. */
export function PropertiesBlock(_props: PropertiesBlockProps) {
  return <section aria-label="Properties" />;
}
