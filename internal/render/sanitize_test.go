package render

import (
	"strings"
	"testing"
)

// TestSanitize is the sanitizer's table (plan 0005, Phase 4): what script,
// handlers, dangerous URLs, styles, forms, frames and foreign content try,
// and what must stay.
func TestSanitize(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain text stays as written", `a &amp; b &lt; c &#160; d`, `a &amp; b &lt; c &#160; d`},
		{"allowed markup stays", `<p class="x">hi <em>there</em><br></p>`, `<p class="x">hi <em>there</em><br></p>`},
		{"script goes with its content", `a<script>alert(1)</script>b`, `ab`},
		{"script in caps", `a<SCRIPT SRC="x.js"></SCRIPT>b`, `ab`},
		{"an unclosed script takes the rest", `a<script>alert(1)`, `a`},
		{"style goes with its content", `<style>body{display:none}</style>x`, `x`},
		{"handlers go", `<img src="https://e.test/a.png" onerror="alert(1)" alt="a">`, `<img src="https://e.test/a.png" alt="a">`},
		{"handlers in any case", `<p OnClick="x()" onmouseover='y()'>t</p>`, `<p>t</p>`},
		{"style attributes go", `<span style="position:fixed;inset:0">x</span>`, `<span>x</span>`},
		{"javascript: links lose their href", `<a href="javascript:alert(1)">x</a>`, `<a>x</a>`},
		{"obfuscated javascript:", `<a href=" jav&#x09;ascript:alert(1)">x</a><a href="JaVaScRiPt:x">y</a>`, `<a>x</a><a>y</a>`},
		{"vbscript: and data:", `<a href="vbscript:x">a</a><a href="data:text/html,<script>x</script>">b</a>`, `<a>a</a><a>b</a>`},
		{"data: images go", `<img src="data:image/svg+xml;base64,AAAA" alt="x">`, `<img alt="x">`},
		{"relative URLs go", `<a href="Notes/Plan.md">a</a><img src="/etc/passwd">`, `<a>a</a><img>`},
		{"protocol-relative URLs go", `<a href="//evil.test/">a</a>`, `<a>a</a>`},
		{"web, mail and fragment links stay", `<a href="https://e.test/?a=1&amp;b=2" rel="noopener">a</a><a href="mailto:x@e.test">m</a><a href="#fn:1">1</a>`,
			`<a href="https://e.test/?a=1&amp;b=2" rel="noopener">a</a><a href="mailto:x@e.test">m</a><a href="#fn:1">1</a>`},
		{"mailto images go", `<img src="mailto:x@e.test">`, `<img>`},
		{"frames and objects go whole", `<iframe src="https://e.test"></iframe><object data="x"><param name="a"></object><embed src="x">t`, `t`},
		{"forms and their fields go whole", `<form action="x"><input type="text" name="a"><button>go</button><select><option>1</option></select><textarea>t</textarea></form>z`, `z`},
		{"an input outside a form goes unless a checkbox", `<input type="password"><input type="checkbox" checked disabled data-line="3" name="x">`,
			`<input type="checkbox" checked="" disabled="" data-line="3">`},
		{"svg and math go whole", `<svg onload="x()"><script>a</script><circle/></svg><math><mi>x</mi></math>k`, `k`},
		{"meta refresh, base and link go", `<meta http-equiv="refresh" content="0;url=x"><base href="https://e.test/"><link rel="stylesheet" href="x.css">k`, `k`},
		{"templates and noscript go whole", `<template><img src="https://e.test/a.png"></template><noscript><p>n</p></noscript>k`, `k`},
		{"unknown elements go, their content stays", `<custom-el data-x="1"><font color="red">x</font></custom-el><center>c</center>`, `xc`},
		{"comments go", `a<!-- <script>x</script> -->b<!-->c`, `abc`},
		{"attributes are escaped", `<p title='a"b&lt;c'>t</p>`, `<p title="a&#34;b&lt;c">t</p>`},
		{"srcset goes", `<img src="https://e.test/a.png" srcset="javascript:x 1x">`, `<img src="https://e.test/a.png">`},
		{"media keep controls, lose their sources", `<video controls src="https://e.test/v.mp4" autoplay data-blob="h.mp4"></video>`, `<video controls="" data-blob="h.mp4"></video>`},
		{"table attributes", `<table><tr><td align="left" colspan="2" bgcolor="red" rowspan="1">x</td></tr></table>`,
			`<table><tr><td align="left" colspan="2" rowspan="1">x</td></tr></table>`},
		{"self-closing tags are written open", `<br/><hr />`, `<br><hr>`},
		{"stray end tags of what goes are dropped", `a</script></style></iframe>b`, `ab`},
		{"namespaced attributes go", `<a xlink:href="javascript:x" href="https://e.test">a</a>`, `<a href="https://e.test">a</a>`},
	} {
		if got := Sanitize(tc.in); got != tc.want {
			t.Errorf("%s:\nSanitize(%q)\n got %q\nwant %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestSanitizeKeepsTheRenderersHTML: every fixture's HTML sanitizes to
// itself, but R02's onclick (docs/specs/render.md#sanitizer).
func TestSanitizeKeepsTheRenderersHTML(t *testing.T) {
	_, fixtures := readRenderSpec(t)
	for _, fx := range fixtures {
		want := fx.Want
		if strings.HasPrefix(fx.Name, "R02 ") {
			want = strings.Replace(want, ` onclick="bad()"`, "", 1)
		}
		if got := Sanitize(fx.Want); got != want {
			t.Errorf("%s:\n got %q\nwant %q", fx.Name, got, want)
		}
	}
}

func TestRenderSanitizes(t *testing.T) {
	got := Render([]byte("Hi <script>alert(1)</script>[[x]] <img src=x onerror=alert(1)>\n"), Options{Path: "a.md"})
	if strings.Contains(got, "script") || strings.Contains(got, "onerror") || !strings.Contains(got, `class="internal-link is-unresolved"`) {
		t.Fatalf("Render = %q", got)
	}
}
