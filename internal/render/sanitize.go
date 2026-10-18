package render

import stdhtml "html"

// Sanitize keeps only the elements and attributes of the spec's allowlist
// (docs/specs/render.md#sanitizer). M3's Phase 4 builds it; until then it
// escapes everything, so nothing it returns can run, and note.render
// answers unavailable.
func Sanitize(html string) string {
	return stdhtml.EscapeString(html)
}
