//go:build integration

package webevidence_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// evil is a note whose HTML and links would run script anywhere that let
// them: each sets window.__xss.
const evil = `# Evil

<script>window.__xss = "script"</script>
<img src="x" onerror="window.__xss = 'onerror'">
<svg onload="window.__xss = 'svg'"><circle r="1"/></svg>
<iframe srcdoc="<script>parent.__xss = 'srcdoc'</script>"></iframe>
<details open ontoggle="window.__xss = 'ontoggle'"><summary>more</summary>text</details>
<a href="javascript:window.__xss='href'">an html link</a>

[a markdown link](javascript:window.__xss='markdown')

![[evil.svg]]
`

// evilSVG would run script when opened as a document.
const evilSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>window.__xss = "svg document"</script><rect width="10" height="10"/></svg>`

// TestIRenderedNotesRunNoScript is Phase 6's XSS check in Firefox, inside
// lodge's development build (plan 0006): a note's HTML and links run no
// script in live preview or in reading view, in notes' frame or the
// shell, and an SVG that carries a script runs none when its content URL
// is opened, since lodge-content serves it sandboxed.
func TestIRenderedNotesRunNoScript(t *testing.T) {
	w := startWorld(t)
	for name, text := range map[string]string{"Evil.md": evil, "evil.svg": evilSVG} {
		if err := os.WriteFile(filepath.Join(w.hub.Dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := startBrowser(t, "xss", 1280, 800)
	clean := `return window.__xss === undefined && window.top.__xss === undefined;`
	// stays checks for a while that nothing ran: script that runs, runs
	// as the page draws.
	stays := func(where string) {
		t.Helper()
		if holds(1500*time.Millisecond, func() bool { return !b.truthy(clean) }) {
			var got string
			b.script(&got, `return String(window.__xss ?? window.top.__xss);`)
			t.Fatalf("%s: script ran (%s)", where, got)
		}
	}

	w.openNote(t, b, "Evil.md", "Evil")
	b.chord(keyControl, keyHome)
	// The embedded SVG draws, from lodge-content, as an image.
	b.await(15*time.Second, "the embedded SVG", func() bool {
		return b.truthy(`return [...document.querySelectorAll(".cm-content img")].some(
			(img) => img.src.startsWith(arguments[0]) && img.src.includes(".svg") && img.complete && img.naturalWidth > 0);`, w.content)
	})
	stays("live preview")
	for _, css := range []string{`.cm-content a[href^="javascript"]`, `.cm-pv-link`} {
		if el, err := b.find(css); err == nil && el != "" {
			b.click(el)
		}
	}
	stays("live preview, links clicked")

	// Reading view.
	leaveEditor(b)
	b.click(b.waitFind(`button[aria-label="Reading view"]`, 10*time.Second))
	b.await(15*time.Second, "the reading view", func() bool {
		return b.truthy(`const r = document.querySelector("[data-testid=reading-view]"); return !!r && r.textContent.includes("an html link");`)
	})
	stays("reading view")
	if el, err := b.find(`a[href^="javascript"]`); err == nil && el != "" {
		t.Fatal("reading view kept a javascript: link")
	}

	// The SVG's own document, opened at its content URL.
	var svg string
	b.script(&svg, `const img = [...document.querySelectorAll("img")].find((i) => i.src.includes(".svg")); return img ? img.src : "";`)
	if svg == "" {
		w.openNote(t, b, "Evil.md", "Evil")
		b.chord(keyControl, keyHome)
		b.script(&svg, `const img = [...document.querySelectorAll(".cm-content img")].find((i) => i.src.includes(".svg")); return img ? img.src : "";`)
	}
	if svg == "" {
		t.Fatal("no content URL for evil.svg")
	}
	b.top()
	b.navigate(svg)
	if holds(1500*time.Millisecond, func() bool { return !b.truthy(`return window.__xss === undefined;`) }) {
		t.Fatal("the SVG's script ran when its content URL was opened")
	}
	t.Logf("no script ran: live preview, reading view, the links, and %s opened as a document", "evil.svg")
}
