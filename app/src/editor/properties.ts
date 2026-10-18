// A note's properties as the YAML text of its frontmatter
// (docs/design/app.md#properties). Task T-0076 replaces these stubs.

/** PropertyValue is a value YAML can hold. */
export type PropertyValue = string | number | boolean | null | PropertyValue[] | { [key: string]: PropertyValue };

/** PropertyType is a property's type, as Obsidian names them. */
export type PropertyType = "text" | "multitext" | "number" | "checkbox" | "date" | "datetime" | "aliases" | "tags";

/** Property is a key of the frontmatter and its value. */
export interface Property {
  key: string;
  value: PropertyValue;
}

/** Frontmatter is what a note's frontmatter holds: nothing, a mapping's properties in order, or why it is not one. */
export type Frontmatter =
  | { kind: "none" }
  | { kind: "properties"; properties: Property[] }
  | { kind: "error"; message: string };

/** readFrontmatter reads a note's properties in order. */
export function readFrontmatter(_text: string): Frontmatter {
  return { kind: "none" };
}

/** setProperties returns text with each key of props set to its value, a key whose value is null removed; an Error says why the frontmatter cannot take it. */
export function setProperties(text: string, _props: Readonly<Record<string, PropertyValue>>): string | Error {
  return text;
}

/** renameProperty returns text with key from renamed to, its value kept, in its place. */
export function renameProperty(text: string, _from: string, _to: string): string | Error {
  return text;
}

/** propertyType is a property's type: the vault's type for its key, else what the key or value says. */
export function propertyType(_key: string, _value: PropertyValue, _vaultType = ""): PropertyType {
  return "text";
}
