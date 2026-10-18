package markdown

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestRenderSource(t *testing.T) {
	for _, tc := range []struct {
		note, want string
		line       int
	}{
		{"Plain.\n", "Plain.\n", 1},
		{"---\ntags: [a]\n---\nBody %%hidden%% here.\n", "Body  here.\n", 4},
		{"\xef\xbb\xbfBOM\n", "BOM\n", 1},
		{"A\n%%\nblock\ncomment\n%%\nB\n", "A\n\n\n\n\nB\n", 1},
		{"`%%code%%` stays %%a%% and %%b%%\n", "`%%code%%` stays  and \n", 1},
		{"Open %%to the end\n", "Open \n", 1},
	} {
		got, line := RenderSource([]byte(tc.note))
		if string(got) != tc.want || line != tc.line {
			t.Errorf("RenderSource(%q) = %q, %d; want %q, %d", tc.note, got, line, tc.want, tc.line)
		}
	}
}

func TestNodeAccessors(t *testing.T) {
	src := []byte("See [[Plan#Goals|the plan]], #garden/beds, $x^2$ and $$y$$.\n\n$$\na+b\n$$\n")
	root := Goldmark().Parser().Parse(text.NewReader(src))
	var links []Link
	var tags, tex []string
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case WikilinkKind():
			l, _ := WikilinkOf(n)
			links = append(links, l)
		case TagKind():
			name, _ := TagOf(n)
			tags = append(tags, name)
		case InlineMathKind(), MathBlockKind():
			s, _ := MathOf(n, src)
			tex = append(tex, s)
		}
		return ast.WalkContinue, nil
	})
	if len(links) != 1 || links[0] != (Link{Kind: LinkWiki, Target: "Plan", Subpath: "#Goals", Alias: "the plan"}) {
		t.Errorf("links = %+v", links)
	}
	if len(tags) != 1 || tags[0] != "garden/beds" {
		t.Errorf("tags = %q", tags)
	}
	if want := []string{"x^2", "y", "a+b"}; len(tex) != 3 || tex[0] != want[0] || tex[1] != want[1] || tex[2] != want[2] {
		t.Errorf("tex = %q, want %q", tex, want)
	}
	if _, ok := WikilinkOf(root); ok {
		t.Error("WikilinkOf(document) ok")
	}
}
