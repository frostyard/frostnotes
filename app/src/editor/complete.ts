// Link and tag completion over the vault catalog (docs/design/app.md#extensions),
// ranked with the quick switcher's fuzzy search.
import {
  type Completion,
  type CompletionContext,
  type CompletionSource,
  pickedCompletion,
} from "@codemirror/autocomplete";
import { syntaxTree } from "@codemirror/language";

import { prepareFuzzySearch, sortByScore } from "../lib/fuzzy";
import { stripMd, switcherResults } from "../lib/switcher";

/** COMPLETION_LIMIT is the most options a completion lists. */
export const COMPLETION_LIMIT = 50;

/** LinkFile is a file a link may name. */
export interface LinkFile {
  /** Its vault path. */
  path: string;
  /** A note's aliases; [] for other files. */
  aliases: readonly string[];
}

/** CompletionCatalog is what completion knows of the vault. */
export interface CompletionCatalog {
  /** Every file, by path. */
  files(): readonly LinkFile[];
  /** What a link to path writes as its target, by the vault's link format: "Plan", "Orchard/Plan", "map.png". */
  linkText(path: string): string;
  /** The headings of the note a target leads to ("" is the note being edited); [] when it leads nowhere. */
  headings(target: string): Promise<readonly { level: number; text: string }[]>;
  /** The block IDs of the note a target leads to, each with its line's text. */
  blocks(target: string): Promise<readonly { id: string; text: string }[]>;
  /** The vault's tags, without "#", with how many notes have each. */
  tags(): readonly { name: string; count: number }[];
}

/** linkCompletion completes a wikilink's target, heading or block ID. */
export function linkCompletion(catalog: CompletionCatalog): CompletionSource {
  return async (context) => {
    if (excluded(context)) return null;
    const query = /\[\[([^[\]|]*)$/.exec(lineBeforeCursor(context))?.[1];
    if (query === undefined) return null;
    const from = context.pos - query.length;
    const hash = query.indexOf("#");
    if (hash !== -1) {
      const target = query.slice(0, hash);
      const block = query[hash + 1] === "^";
      const offset = hash + (block ? 2 : 1);
      const options = block
        ? (await catalog.blocks(target)).map(({ id, text }) => linkOption(id, id, text))
        : (await catalog.headings(target)).map(({ text }) => linkOption(text, text));
      return { from: from + offset, options: rankOptions(options, query.slice(offset)), filter: false };
    }
    const files = catalog.files();
    const options = query.trim()
      ? switcherResults(files, query, []).map((result) =>
          result.kind === "alias"
            ? linkOption(result.alias, `${catalog.linkText(result.path)}|${result.alias}`, stripMd(result.path))
            : linkOption(stripMd(result.path), catalog.linkText(result.path)),
        )
      : files.slice(0, COMPLETION_LIMIT).map(({ path }) => linkOption(stripMd(path), catalog.linkText(path)));
    return { from, options: options.slice(0, COMPLETION_LIMIT), filter: false };
  };
}

/** tagCompletion completes a tag after its "#". */
export function tagCompletion(catalog: CompletionCatalog): CompletionSource {
  return (context) => {
    if (excluded(context)) return null;
    const query = /(?:^|\s)#([\p{L}\p{M}\p{Nd}_/-]+)$/u.exec(lineBeforeCursor(context))?.[1];
    if (query === undefined) return null;
    const search = prepareFuzzySearch(query);
    const matches = catalog.tags().flatMap((tag) => {
      const match = search(tag.name);
      return match ? [{ ...tag, match }] : [];
    });
    matches.sort((a, b) => b.match.score - a.match.score || b.count - a.count || a.name.localeCompare(b.name));
    return {
      from: context.pos - query.length,
      options: matches.slice(0, COMPLETION_LIMIT).map(({ name, count }) => ({
        label: name,
        apply: name,
        detail: String(count),
      })),
      filter: false,
    };
  };
}

function excluded(context: CompletionContext): boolean {
  for (let node = syntaxTree(context.state).resolveInner(context.pos, -1); node; ) {
    if (/^(InlineCode|FencedCode|CodeBlock|InlineMath|BlockMath|HTMLBlock|CommentBlock)$/.test(node.name)) {
      return true;
    }
    if (!node.parent) break;
    node = node.parent;
  }
  return false;
}

function lineBeforeCursor(context: CompletionContext): string {
  const line = context.state.doc.lineAt(context.pos);
  return line.text.slice(0, context.pos - line.from);
}

function linkOption(label: string, text: string, detail?: string): Completion {
  return {
    label,
    detail,
    apply(view, completion, from, to) {
      const closed = view.state.doc.sliceString(to, to + 2) === "]]";
      view.dispatch({
        changes: { from, to, insert: closed ? text : `${text}]]` },
        selection: { anchor: from + text.length + 2 },
        annotations: pickedCompletion.of(completion),
        userEvent: "input.complete",
      });
    },
  };
}

function rankOptions(options: readonly Completion[], query: string): Completion[] {
  if (!query.trim()) return options.slice(0, COMPLETION_LIMIT);
  const search = prepareFuzzySearch(query);
  const matches = options.flatMap((option) => {
    const match = search(option.label);
    return match ? [{ option, match }] : [];
  });
  return sortByScore(matches)
    .slice(0, COMPLETION_LIMIT)
    .map(({ option }) => option);
}
