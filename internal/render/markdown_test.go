package render

import (
	"strings"
	"testing"
)

// mdVault is a small vault for the Markdown format's tests.
func mdVault() Options {
	notes := map[string]string{
		"Notes/Plan.md":  "# Plan\n\n## Goals\n\nGrow [[Seeds]].\n\n## Later\n\nRest.\n",
		"Notes/Seeds.md": "Seeds embed ![[Plan#Goals]]\n",
		"Notes/Loop.md":  "![[Loop]]\n",
		"Notes/Deep1.md": "![[Deep2]]\n",
		"Notes/Deep2.md": "![[Deep3]]\n",
		"Notes/Deep3.md": "![[Deep4]]\n",
		"Notes/Deep4.md": "deepest\n",
	}
	files := map[string]string{"Files/photo.png": "aaaa", "Files/doc.pdf": "bbbb"}
	for p := range notes {
		files[p] = "h-" + p
	}
	return Options{
		Path: "Notes/Here.md",
		Resolve: func(from, target string) string {
			for p := range files {
				base := p[strings.LastIndexByte(p, '/')+1:]
				if base == target || strings.TrimSuffix(base, ".md") == target || p == target {
					return p
				}
			}
			return ""
		},
		File: func(p string) (string, bool) { h, ok := files[p]; return h, ok },
		Note: func(p string) ([]byte, bool) { n, ok := notes[p]; return []byte(n), ok },
	}
}

func TestMarkdownFormat(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"frontmatter goes", "---\ntags: [a]\n---\nBody\n", "Body\n"},
		{"wikilink", "See [[Plan]].\n", "See [Plan](<note:Notes/Plan.md>).\n"},
		{"alias and heading", "[[Plan#Goals|our goals]] and [[Plan#Later]]\n",
			"[our goals](<note:Notes/Plan.md#Goals>) and [Plan > Later](<note:Notes/Plan.md#Later>)\n"},
		{"unresolved", "[[Nowhere]]\n", "[Nowhere](<note:Nowhere>)\n"},
		{"own heading", "[[#Top]]\n", "[Top](<note:#Top>)\n"},
		{"tag", "A #garden/beds tag\n", "A [#garden/beds](<tag:garden/beds>) tag\n"},
		{"highlight", "an ==important== word\n", "an <mark>important</mark> word\n"},
		{"comment goes", "kept %%hidden%% kept\n", "kept  kept\n"},
		{"block id goes", "a line ^abc-1\n", "a line\n"},
		{"image embed", "![[photo.png|300]]\n", "![photo.png](<blob:aaaa.png>)\n"},
		{"file embed", "![[doc.pdf]]\n", "[doc.pdf](<note:Files/doc.pdf>)\n"},
		{"embed in text is a link", "see ![[Plan]] here\n", "see [Plan](<note:Notes/Plan.md>) here\n"},
		{"code stays", "`[[Plan]] #tag ==x==`\n\n```\n[[Plan]] #tag\n```\n",
			"`[[Plan]] #tag ==x==`\n\n```\n[[Plan]] #tag\n```\n"},
		{"spaces in a path", "[[My note]]\n", "[My note](<note:My note>)\n"},
		{"markdown link", "a [plan](Plan.md#Goals \"t\") b\n", "a [plan](<note:Notes/Plan.md#Goals> \"t\") b\n"},
		{"markdown image", "![a photo](photo.png)\n", "![a photo](<blob:aaaa.png>)\n"},
		{"web link stays", "[site](https://example.com/a_(b))\n", "[site](https://example.com/a_(b))\n"},
	}
	for _, c := range cases {
		if got := Markdown([]byte(c.in), mdVault()); got != c.want {
			t.Errorf("%s:\n%q\ngives\n%q\nwant\n%q", c.name, c.in, got, c.want)
		}
	}
}

func TestMarkdownNoteEmbeds(t *testing.T) {
	got := Markdown([]byte("Before\n\n![[Plan#Goals]]\n\nAfter\n"), mdVault())
	want := "Before\n\n```frostnotes-embed Notes/Plan.md#Goals\n## Goals\n\nGrow [Seeds](<note:Notes/Seeds.md>).\n\n```\n\nAfter\n"
	if got != want {
		t.Fatalf("section embed:\n%q\nwant\n%q", got, want)
	}
	// A loop is a link at once; a chain stops three levels down.
	if got := Markdown([]byte("![[Loop]]\n"), withPath(mdVault(), "Notes/Loop.md")); !strings.Contains(got, "[Loop](<note:Notes/Loop.md>)") {
		t.Errorf("a loop embeds as %q", got)
	}
	deep := Markdown([]byte("![[Deep1]]\n"), mdVault())
	if strings.Contains(deep, "deepest") || !strings.Contains(deep, "[Deep4](<note:Notes/Deep4.md>)") {
		t.Errorf("a deep chain renders as %q", deep)
	}
	if strings.Count(deep, "```") < 6 {
		t.Errorf("the chain's three embeds are not fenced: %q", deep)
	}
}

func withPath(o Options, p string) Options { o.Path = p; return o }

func TestMarkdownCallouts(t *testing.T) {
	in := "> [!Warning]- Mind the [[Plan]]\n> Body with #tag\n> ```\n> code ``` here\n> ```\n\nAfter\n"
	got := Markdown([]byte(in), mdVault())
	want := "````frostnotes-callout warning- Mind the [[Plan]]\nBody with [#tag](<tag:tag>)\n```\ncode ``` here\n```\n````\n\nAfter\n"
	if got != want {
		t.Fatalf("callout:\n%q\nwant\n%q", got, want)
	}
	if got := Markdown([]byte("> plain quote [[Plan]]\n"), mdVault()); got != "> plain quote [Plan](<note:Notes/Plan.md>)\n" {
		t.Errorf("a plain quote: %q", got)
	}
}

func TestNoteLink(t *testing.T) {
	p, sub, tag, ok := NoteLink("note:Notes/My%20plan.md#Goals")
	if !ok || p != "Notes/My plan.md" || sub != "#Goals" || tag != "" {
		t.Errorf("note link: %q %q %q %v", p, sub, tag, ok)
	}
	if _, _, tag, ok := NoteLink("tag:garden"); !ok || tag != "garden" {
		t.Errorf("tag link: %q %v", tag, ok)
	}
	if _, _, _, ok := NoteLink("https://example.com"); ok {
		t.Error("a web link read as a note")
	}
}

func TestMarkdownReferenceLinkBeforeAnInlineLink(t *testing.T) {
	in := "[plan][p] and [site](https://example.com)\n\n[p]: Plan.md\n"
	got := Markdown([]byte(in), mdVault())
	if !strings.Contains(got, "[site](https://example.com)") {
		t.Fatalf("the inline link changed: %q", got)
	}
}

func TestMarkdownBlocksStayInTheirContainers(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"an embed in a quote", "> ![[Deep4]]\n\nafter\n", "> ```frostnotes-embed Notes/Deep4.md\n> deepest\n> ```\n\nafter\n"},
		{"a callout in a list", "- item\n\n  > [!note] T\n  > body\n", "- item\n\n  ```frostnotes-callout note T\n  body\n  ```\n"},
		{"a callout in a quote", "> > [!note]\n> > body\n", "> ```frostnotes-callout note\n> body\n> ```\n"},
	} {
		if got := Markdown([]byte(tc.in), mdVault()); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMarkdownFenceWithABacktickInItsInfo(t *testing.T) {
	got := Markdown([]byte("> [!note] Use `x` here\n> a ~~~ b\n\nafter\n"), mdVault())
	want := "~~~~frostnotes-callout note Use `x` here\na ~~~ b\n~~~~\n\nafter\n"
	if got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	if out := HTML([]byte(got), Options{}); !strings.Contains(out, "<p>after</p>") {
		t.Fatalf("what follows the block is not a paragraph: %q", out)
	}
}

func TestMarkdownImageWithNoAlt(t *testing.T) {
	got := Markdown([]byte("![](photo.png) and ![](photo.png)\n"), mdVault())
	if want := "![](<blob:aaaa.png>) and ![](<blob:aaaa.png>)\n"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}

func TestAnEscapedHashIsPartOfTheName(t *testing.T) {
	l, ok := markdownDestination([]byte("draft%231.png#part"))
	if !ok || l.Target != "draft#1.png" || l.Subpath != "#part" {
		t.Fatalf("%+v", l)
	}
}

func TestAReferenceImageTakesNoOtherImagesDestination(t *testing.T) {
	for _, in := range []string{
		"![][r]\n\n![](b.png)\n\n[r]: photo.png\n",
		"![][r] ![](b.png)\n\n[r]: photo.png\n",
	} {
		got := Markdown([]byte(in), mdVault())
		if !strings.Contains(got, "![](b.png)") || strings.Contains(got, "![](<blob:aaaa.png>)") {
			t.Errorf("%q made %q", in, got)
		}
	}
	for _, in := range []string{"![][r] `![](photo.png)`\n\n[r]: photo.png\n", "![][r]`![](photo.png)`\n\n[r]: photo.png\n"} {
		if got := Markdown([]byte(in), mdVault()); strings.Contains(got, "`![](<blob") {
			t.Errorf("the code span was rewritten: %q", got)
		}
	}
	got := Markdown([]byte("![][r] ![](photo.png)\n\n[r]: doc.pdf\n"), mdVault())
	if !strings.Contains(got, "![](<blob:aaaa.png>)") {
		t.Errorf("the image after a reference was not rewritten: %q", got)
	}
}

func TestAdjacentReferenceImagesCannotRewriteCode(t *testing.T) {
	for _, in := range []string{
		"![][a]![][b] `![](photo.png)`\n\n[a]: photo.png\n[b]: photo.png\n",
		"![][a]![][a] `![](photo.png)` ![](photo.png)\n\n[a]: photo.png\n",
	} {
		got := Markdown([]byte(in), mdVault())
		if !strings.Contains(got, "`![](photo.png)`") {
			t.Fatalf("code span rewritten: %q", got)
		}
		if strings.Contains(in, "` ![]") && !strings.Contains(got, "![](<blob:aaaa.png>)") {
			t.Fatalf("real image not rewritten: %q", got)
		}
	}
}
