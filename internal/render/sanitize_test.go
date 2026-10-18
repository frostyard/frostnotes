package render

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestSanitize is the sanitizer's table (plan 0005, Phase 4): what script,
// handlers, dangerous URLs, styles, forms, frames and foreign content try,
// and what must stay.
func TestSanitize(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain text stays as written", `a &amp; b &lt; c &#160; d`, `a &amp; b &lt; c &#160; d`},
		{"allowed markup stays", `<p class="callout">hi <em>there</em><br></p>`, `<p class="callout">hi <em>there</em><br></p>`},
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
		// A browser ignores the slash of a non-void HTML element (review finding 1, plan 0005 Phase 7).
		{"a self-closing xmp takes the rest", `a<xmp/><img src=x onerror=y>b`, `a`},
		{"a self-closing iframe goes with its content", `a<iframe/><b>x</b></iframe>c`, `ac`},
		{"a self-closing script goes with its content", `a<script/>alert(1)</script><img src=x onerror=alert(2)>c`, `a<img>c`},
		{"a self-closing form goes with its fields", `a<form/><input type="text"><b>x</b></form>c`, `ac`},
		{"self-closing svg and math are empty", `<svg/>a<math/>b`, `ab`},
		{"a stray < cannot join text into a tag", `<<x>script>alert(1)<<x>/script>a < b`, `&lt;script>alert(1)&lt;/script>a &lt; b`},
		// Only the renderer's classes, footnotes' ids and roles stay (review finding 4).
		{"classes the app styles go", `<div class="fixed inset-0 z-50 callout bg-backdrop">x</div><a class="external-link fixed" href="https://e.test">y</a>`,
			`<div class="callout">x</div><a class="external-link" href="https://e.test">y</a>`},
		{"a class with nothing left goes", `<span class="fixed inset-0">x</span><code class="language-go">y</code>`, `<span>x</span><code class="language-go">y</code>`},
		{"ids other than footnotes' go", `<span id="isTauri">x</span><sup id="fnref:1">1</sup><li id="fn:1">n</li>`, `<span>x</span><sup id="fnref:1">1</sup><li id="fn:1">n</li>`},
		{"roles other than footnotes' go", `<div role="dialog">x</div><a role="doc-noteref" href="#fn:1">1</a>`, `<div>x</div><a role="doc-noteref" href="#fn:1">1</a>`},
	} {
		if got := Sanitize(tc.in); got != tc.want {
			t.Errorf("%s:\nSanitize(%q)\n got %q\nwant %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestSanitizeKeepsTheRenderersHTML: every fixture's HTML sanitizes to
// itself, but R02's class and onclick (docs/specs/render.md#sanitizer).
func TestSanitizeKeepsTheRenderersHTML(t *testing.T) {
	_, fixtures := readRenderSpec(t)
	for _, fx := range fixtures {
		want := fx.Want
		if strings.HasPrefix(fx.Name, "R02 ") {
			want = strings.Replace(want, ` class="x" onclick="bad()"`, "", 1)
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

// TestSanitizeRawTextSelfClosing: each element the tokenizer reads as raw
// text goes with what follows it when written self-closing.
func TestSanitizeRawTextSelfClosing(t *testing.T) {
	for _, name := range []string{"script", "style", "textarea", "title", "xmp", "iframe", "noembed", "noframes", "noscript", "plaintext"} {
		in := "a<" + name + "/><img src=x onerror=alert(1)><a href=\"javascript:x\">y</a></" + name + ">b"
		want := "ab"
		if name == "plaintext" {
			want = "a" // plaintext never ends
		}
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSanitizedHTMLParsesClean reads the sanitizer's output as a browser
// does (the HTML tree builder) and finds nothing off the allowlist, for
// the table's inputs and the fixtures; FuzzSanitize does the same for
// any input.
func TestSanitizedHTMLParsesClean(t *testing.T) {
	_, fixtures := readRenderSpec(t)
	ins := []string{
		`<xmp/><img src=x onerror=y>`, `<iframe/><svg onload=x>`, `<textarea/></textarea><img src=x onerror=y>`,
		`<table><td><style/><img onerror=x></style>`, `<noscript/><p onclick=x>`, `<math><mi xlink:href="javascript:x">y</mi></math>`,
		`<a href="java&#0;script:x">a</a>`, `<select><option><img src=x onerror=y></select>`, `<svg/><img src=x onerror=y>`,
	}
	for _, fx := range fixtures {
		ins = append(ins, fx.Want)
	}
	for _, in := range ins {
		checkClean(t, in, Sanitize(in))
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range []string{`<xmp/><img src=x onerror=y>`, `<p class="x" id="y">t</p>`, `<a href="https://e.test">a</a>`, `<svg/><b>x</b>`, `<<A0>C>`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkClean(t, in, Sanitize(in))
	})
}

// checkClean fails when out, parsed as a browser parses it, holds an
// element or attribute the sanitizer would not keep.
func checkClean(t *testing.T, in, out string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data == "html" || n.Data == "head" || n.Data == "body" {
			continue
		}
		if !allowedElement(n.Data) || dropsContent(html.Token{Data: n.Data, Attr: n.Attr}) || n.Namespace != "" {
			t.Errorf("Sanitize(%q) = %q: a browser reads a %s %s", in, out, n.Namespace, n.Data)
		}
		for _, a := range n.Attr {
			if a.Namespace != "" || !allowedAttr(n.Data, a.Key, a.Val) || a.Key == "class" && classes(a.Val) != a.Val {
				t.Errorf("Sanitize(%q) = %q: a browser reads %s's %s=%q", in, out, n.Data, a.Key, a.Val)
			}
		}
	}
}
