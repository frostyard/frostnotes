// Package panes draws the desktop app's sidebars' panes
// (docs/specs/ui.md#sidebars-and-panes, plan 0009): Files, Search,
// Backlinks, Outgoing links, Tags, All properties, Outline and Tasks. They
// read notesd through their Env and ask the window to act through Actions.
package panes

import (
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/frostyard/frostnotes/api"
)

// The Files pane's sort orders, by Obsidian's names.
const (
	SortAlphabetical        = "alphabetical"
	SortAlphabeticalReverse = "alphabeticalReverse"
	SortModified            = "byModifiedTime"
	SortModifiedReverse     = "byModifiedTimeReverse"
)

// File types, as Obsidian makes them of a file's extension.
var fileTypes = map[string]string{
	"md": "note", "canvas": "canvas", "base": "base", "pdf": "pdf",
	"bmp": "image", "png": "image", "jpg": "image", "jpeg": "image", "gif": "image", "svg": "image", "webp": "image", "avif": "image",
	"mp3": "audio", "wav": "audio", "m4a": "audio", "3gp": "audio", "flac": "audio", "ogg": "audio", "oga": "audio", "opus": "audio",
	"mp4": "video", "webm": "video", "ogv": "video", "mov": "video", "mkv": "video",
}

// Extension is a path's extension in lower case, without the dot.
func Extension(p string) string {
	name := path.Base(p)
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 {
		return ""
	}
	return strings.ToLower(name[dot+1:])
}

// FileType is what Obsidian makes of a file.
func FileType(p string) string {
	if t, ok := fileTypes[Extension(p)]; ok {
		return t
	}
	return "other"
}

// compareNatural orders names as a collator with numbers does: ignoring
// case, and runs of digits by their value.
func compareNatural(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			x := strings.TrimLeft(string(ra[si:i]), "0")
			y := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(x) != len(y) {
				return len(x) - len(y)
			}
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
			continue
		}
		x, y := unicode.ToLower(ra[i]), unicode.ToLower(rb[j])
		if x != y {
			return int(x) - int(y)
		}
		i++
		j++
	}
	return (len(ra) - i) - (len(rb) - j)
}

// Node is a folder or file of the Files pane.
type Node struct {
	Path string
	// Name is a folder's name, or a file's name without its extension.
	Name string
	// Extension is a file's, in lower case.
	Extension string
	// Type is a file's type; "folder" for a folder.
	Type     string
	Children []string // a folder's children, sorted; nil for a file
	modified int64
	full     string
}

// Tree is a vault's folders and files, nested and sorted.
type Tree struct {
	Nodes map[string]*Node
	Roots []string
}

// BuildTree nests the files and folders, sorted as Obsidian's file
// explorer sorts them: folders first by name, files by the sort order.
// Files of type other are left out unless showUnsupported.
func BuildTree(files []api.VaultFile, folders []string, sort string, showUnsupported bool) *Tree {
	t := &Tree{Nodes: map[string]*Node{"": {Type: "folder"}}}
	var ensure func(p string) *Node
	ensure = func(p string) *Node {
		if n := t.Nodes[p]; n != nil {
			return n
		}
		n := &Node{Path: p, Name: path.Base(p), Type: "folder"}
		t.Nodes[p] = n
		parent := parentOf(p)
		ensure(parent).Children = append(ensure(parent).Children, p)
		return n
	}
	for _, f := range folders {
		ensure(f)
	}
	for _, f := range files {
		typ := FileType(f.Path)
		if typ == "other" && !showUnsupported {
			continue
		}
		base := path.Base(f.Path)
		name := base
		if ext := Extension(f.Path); ext != "" {
			name = base[:len(base)-len(ext)-1]
		}
		t.Nodes[f.Path] = &Node{Path: f.Path, Name: name, Extension: Extension(f.Path), Type: typ, modified: f.ModifiedAt.UnixMilli(), full: base}
		parent := ensure(parentOf(f.Path))
		parent.Children = append(parent.Children, f.Path)
	}
	for _, n := range t.Nodes {
		slices.SortFunc(n.Children, func(a, b string) int { return t.compare(a, b, sort) })
	}
	t.Roots = t.Nodes[""].Children
	return t
}

func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

func (t *Tree) compare(a, b, sort string) int {
	x, y := t.Nodes[a], t.Nodes[b]
	fx, fy := x.Type == "folder", y.Type == "folder"
	switch {
	case fx && !fy:
		return -1
	case !fx && fy:
		return 1
	case fx:
		if c := compareNatural(x.Name, y.Name); c != 0 {
			return c
		}
		return strings.Compare(x.Name, y.Name)
	}
	alpha := compareNatural(x.Name, y.Name)
	if alpha == 0 {
		alpha = compareNatural(x.full, y.full)
	}
	if alpha == 0 {
		alpha = strings.Compare(x.full, y.full)
	}
	switch sort {
	case SortAlphabeticalReverse:
		return -alpha
	case SortModified:
		if x.modified != y.modified {
			return int(y.modified - x.modified)
		}
	case SortModifiedReverse:
		if x.modified != y.modified {
			return int(x.modified - y.modified)
		}
	}
	return alpha
}

// TagNode is a tag and the tags nested under it.
type TagNode struct {
	Name     string // the tag's last segment, as first spelled
	Tag      string // the whole tag, without '#'
	Count    int64  // the notes with the tag, plus its children's
	Children []*TagNode
}

// TagTree nests tag counts into a tree, sorted by name or by count at
// every level.
func TagTree(counts []api.TagCount, byCount bool) []*TagNode {
	var roots []*TagNode
	byPath := map[string]*TagNode{}
	for _, tc := range counts {
		var parent *TagNode
		for name := range strings.SplitSeq(tc.Tag, "/") {
			if name == "" {
				continue
			}
			p := name
			if parent != nil {
				p = parent.Tag + "/" + name
			}
			key := strings.ToLower(p)
			n := byPath[key]
			if n == nil {
				n = &TagNode{Name: name, Tag: p}
				byPath[key] = n
				if parent != nil {
					parent.Children = append(parent.Children, n)
				} else {
					roots = append(roots, n)
				}
			}
			n.Count += tc.Count
			parent = n
		}
	}
	var sortNodes func([]*TagNode)
	sortNodes = func(ns []*TagNode) {
		slices.SortStableFunc(ns, func(a, b *TagNode) int {
			if byCount && a.Count != b.Count {
				return int(b.Count - a.Count)
			}
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
		for _, n := range ns {
			sortNodes(n.Children)
		}
	}
	sortNodes(roots)
	return roots
}
