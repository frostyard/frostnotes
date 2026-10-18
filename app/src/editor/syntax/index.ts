// The editor's parser: @lezer/markdown with GFM and Frostnotes' extensions
// for Obsidian's syntax (docs/design/app.md#extensions;
// docs/specs/markdown.md), each a file here, checked against the spec's
// fixtures (syntax/fixtures.test.ts). Comments are not syntax nodes but
// ranges (comments.ts), and callouts are read from a quote's first line
// (callouts.ts).
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import type { Extension } from "@codemirror/state";
import type { MarkdownConfig } from "@lezer/markdown";

import { BlockId } from "./blockIds";
import { commentStyle } from "./comments";
import { Embed } from "./embeds";
import { Footnote } from "./footnotes";
import { Frontmatter } from "./frontmatter";
import { Highlight } from "./highlights";
import { MathSyntax } from "./math";
import { Hashtag } from "./tags";
import { WikiLink } from "./wikilink";

/** OBSIDIAN are the extensions for Obsidian's syntax, in the order they apply. */
export const OBSIDIAN: MarkdownConfig[] = [
  Frontmatter,
  WikiLink,
  Embed,
  MathSyntax,
  Hashtag,
  Highlight,
  BlockId,
  Footnote,
];

/** markdownSupport is the editor's language: GFM with Obsidian's syntax, and its comments marked. */
export function markdownSupport(): Extension {
  // Smart lists' keys are a preference (extensions.ts), not the language's.
  return [markdown({ base: markdownLanguage, extensions: OBSIDIAN, addKeymap: false }), commentStyle];
}
