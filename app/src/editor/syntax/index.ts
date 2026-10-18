// The editor's parser: @lezer/markdown with GFM and Frostnotes' extensions
// for Obsidian's syntax (docs/design/app.md#extensions;
// docs/specs/markdown.md). Each extension is a file here, against the
// spec's fixtures (syntax/fixtures.test.ts); batch 3's cards add embeds,
// tags, highlights, comments, callouts, block IDs, math and footnotes to
// OBSIDIAN.
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import type { Extension } from "@codemirror/state";
import type { MarkdownConfig } from "@lezer/markdown";

import { Frontmatter } from "./frontmatter";
import { WikiLink } from "./wikilink";

/** OBSIDIAN are the extensions for Obsidian's syntax, in the order they apply. */
export const OBSIDIAN: MarkdownConfig[] = [Frontmatter, WikiLink];

/** markdownSupport is the editor's language: GFM with Obsidian's syntax. */
export function markdownSupport(): Extension {
  return markdown({ base: markdownLanguage, extensions: OBSIDIAN });
}
