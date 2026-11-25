// Package markup is what the desktop app's editor does to a note's
// Markdown source as plain text (docs/design/app.md#editor): the
// formatting commands and smart lists, the link or tag under the caret,
// the frontmatter and its properties, and the attachment rules (where a
// file goes, its name, its embed). It imports no UI toolkit, neither MyGo
// nor frostui, so a client on another toolkit can use it too
// (internal/markdown is the parser of notesd's index).
package markup
