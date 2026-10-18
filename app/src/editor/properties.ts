// A note's properties as the YAML text of its frontmatter
// (docs/design/app.md#properties), edited by key without rewriting untouched text.

import { isMap, parseDocument, stringify } from "yaml";

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
export function readFrontmatter(text: string): Frontmatter {
  const front = splitFrontmatter(text);
  if (!front.exists) return { kind: "none" };
  const parsed = parseProperties(front.text);
  return parsed instanceof Error
    ? { kind: "error", message: parsed.message }
    : { kind: "properties", properties: parsed.properties };
}

/** setProperties returns text with each key of props set to its value, a key whose value is null removed; an Error says why the frontmatter cannot take it. */
export function setProperties(text: string, props: Readonly<Record<string, PropertyValue>>): string | Error {
  const front = splitFrontmatter(text);
  const parsed = parseProperties(front.text);
  if (parsed instanceof Error) return parsed;
  let added = "";
  for (const key of Object.keys(props).sort()) {
    const value = props[key];
    if (value === undefined) continue;
    const rendered = value === null ? "" : renderProperty(key, value, front.ending);
    if (rendered instanceof Error) return rendered;
    const unit = parsed.units.get(key);
    if (unit) replaceUnit(parsed.lines, unit, rendered);
    else added += rendered;
  }
  return replaceFrontmatter(text, front, parsed.lines.join("") + added);
}

/** renameProperty returns text with key from renamed to, its value kept, in its place. */
export function renameProperty(text: string, from: string, to: string): string | Error {
  if (from === to) return text;
  const front = splitFrontmatter(text);
  const parsed = parseProperties(front.text);
  if (parsed instanceof Error) return parsed;
  const property = parsed.properties.find(({ key }) => key === from);
  if (!property) return new Error(`property ${from} does not exist`);
  if (parsed.properties.some(({ key }) => key === to)) return new Error(`property ${to} already exists`);
  const unit = parsed.units.get(from);
  if (!unit) return new Error(`property ${from} has no editable lines`);
  const rendered = renderProperty(to, property.value, front.ending);
  if (rendered instanceof Error) return rendered;
  replaceUnit(parsed.lines, unit, rendered);
  return replaceFrontmatter(text, front, parsed.lines.join(""));
}

/** propertyType is a property's type: the vault's type for its key, else what the key or value says. */
export function propertyType(key: string, value: PropertyValue, vaultType = ""): PropertyType {
  switch (vaultType) {
    case "text":
    case "multitext":
    case "number":
    case "checkbox":
    case "date":
    case "datetime":
    case "aliases":
    case "tags":
      return vaultType;
  }
  const name = key.toLowerCase();
  if (name === "tags" || name === "aliases") return name;
  if (Array.isArray(value)) return "multitext";
  if (typeof value === "boolean") return "checkbox";
  if (typeof value === "number") return "number";
  if (typeof value === "string") {
    if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return "date";
    if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$/.test(value)) return "datetime";
  }
  return "text";
}

interface FrontmatterText {
  exists: boolean;
  start: number;
  text: string;
  ending: string;
}

interface PropertyUnit {
  start: number;
  end: number;
}

interface ParsedProperties {
  properties: Property[];
  lines: string[];
  units: Map<string, PropertyUnit>;
}

function splitFrontmatter(text: string): FrontmatterText {
  const bom = text.startsWith("\ufeff") ? 1 : 0;
  const match = /^---(\r?\n)(?:([\s\S]*?\r?\n))?---(?:\r?\n|$)/.exec(text.slice(bom));
  const ending = match?.[1] ?? "\n";
  return {
    exists: match !== null,
    start: bom + (match ? 3 + ending.length : 0),
    text: match?.[2] ?? "",
    ending,
  };
}

function parseProperties(text: string): ParsedProperties | Error {
  try {
    const doc = parseDocument(text, { uniqueKeys: true });
    if (doc.errors.length) return new Error(doc.errors.map((error) => error.message).join("\n"));
    const root = doc.contents;
    if (root !== null && (!isMap(root) || root.flow)) {
      return new Error("frontmatter must be a block YAML mapping");
    }
    const lines = text.split(/(?<=\n)/);
    let offset = 0;
    const starts = lines.map((line) => {
      const start = offset;
      offset += line.length;
      return start;
    });
    const values = doc.toJS() as Record<string, PropertyValue> | null;
    const properties: Property[] = [];
    const units = new Map<string, PropertyUnit>();
    for (const pair of root?.items ?? []) {
      const key = String(pair.key.toJS(doc));
      properties.push({ key, value: values?.[key] ?? null });
      const offset = pair.key.range?.[0];
      if (offset === undefined) continue;
      const start = starts.findLastIndex((lineStart) => lineStart <= offset);
      const line = lines[start];
      if (!line || /^[ \t#-]/.test(line)) continue;
      units.set(key, { start, end: propertyUnitEnd(lines, start + 1) });
    }
    return { properties, lines, units };
  } catch (error) {
    return propertyError(error);
  }
}

function propertyUnitEnd(lines: string[], start: number): number {
  while (start < lines.length) {
    const line = lines[start] ?? "";
    if (line.trim() === "") {
      // Keep trailing blank lines; consume them only when the value resumes.
      let next = start + 1;
      while (next < lines.length && lines[next]?.trim() === "") next++;
      if (!/^[ \t-]/.test(lines[next] ?? "")) break;
      start = next;
    } else if (/^[ \t-]/.test(line)) {
      start++;
    } else {
      break;
    }
  }
  return start;
}

function replaceUnit(lines: string[], unit: PropertyUnit, rendered: string): void {
  lines[unit.start] = rendered;
  for (let i = unit.start + 1; i < unit.end; i++) lines[i] = "";
}

function renderProperty(key: string, value: PropertyValue, ending: string): string | Error {
  try {
    return stringify({ [key]: value }, { indent: 2, lineWidth: 0 }).replaceAll("\n", ending);
  } catch (error) {
    return propertyError(error);
  }
}

function replaceFrontmatter(text: string, front: FrontmatterText, replacement: string): string | Error {
  if (!front.exists && replacement === "") return text;
  const result = front.exists
    ? text.slice(0, front.start) + replacement + text.slice(front.start + front.text.length)
    : `${text.slice(0, front.start)}---\n${replacement}---\n${text.slice(front.start)}`;
  const checked = readFrontmatter(result);
  if (checked.kind === "error") return new Error(checked.message);
  if (checked.kind === "none") return new Error("result has no frontmatter mapping");
  return result;
}

function propertyError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}
