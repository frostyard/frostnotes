// Package markup is what the desktop app's editor does to a note's
// Markdown source as plain text (docs/design/app.md#editor): the
// formatting commands and smart lists, what the text before a caret
// completes to, the link or tag under the caret, the frontmatter and its
// properties, the attachment rules (where a file goes, its name, its
// embed), and what source mode colors and folds, with
// places in UTF-16 columns as NSGo's editor counts them. It imports no UI
// toolkit, neither MyGo nor frostui, so a client on another toolkit can use
// it too (internal/markdown is the parser of notesd's index).
package markup
