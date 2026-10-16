package markdown

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// parseFront sets d's Props, PropError and Aliases and adds the
// frontmatter's tags and property links (docs/specs/markdown.md#frontmatter).
func parseFront(d *Doc, s *source, split Split) {
	if !split.HasFront {
		return
	}
	var document yaml.Node
	if err := yaml.Unmarshal(split.Front, &document); err != nil {
		d.PropError = fmt.Errorf("frontmatter: %w", err)
		return
	}
	if len(document.Content) == 0 {
		d.Props = map[string]any{}
		return
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		d.PropError = fmt.Errorf("frontmatter: expected a mapping")
		return
	}
	decoder := frontDecoder{active: map[*yaml.Node]bool{}, values: map[*yaml.Node]any{}}
	props, err := decoder.value(root)
	if err != nil {
		d.PropError = fmt.Errorf("frontmatter: %w", err)
		return
	}
	d.Props = props.(map[string]any)
	frontLine := s.line(bytes.IndexByte(s.note, '\n') + 1)
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		line := frontLine + key.Line - 1
		switch key.Value {
		case "tags":
			collectFrontTags(d, value, line)
		case "aliases":
			for _, item := range frontScalars(value) {
				if item.Value != "" {
					d.Aliases = append(d.Aliases, item.Value)
				}
			}
		default:
			for _, item := range frontScalars(value) {
				if item.Tag == "!!str" {
					collectPropertyLinks(d, s, item, key.Value, line, frontLine)
				}
			}
		}
	}
}

// Cache completed values so aliases do not repeatedly expand their targets.
// Active nodes detect recursive aliases, which JSON cannot represent.
type frontDecoder struct {
	active map[*yaml.Node]bool
	values map[*yaml.Node]any
}

func (f *frontDecoder) value(n *yaml.Node) (any, error) {
	if f.active[n] {
		return nil, fmt.Errorf("recursive alias %q", n.Anchor)
	}
	if value, ok := f.values[n]; ok {
		return value, nil
	}
	f.active[n] = true
	defer delete(f.active, n)
	var value any
	var err error
	switch n.Kind {
	case yaml.MappingNode:
		value, err = f.mapping(n)
	case yaml.SequenceNode:
		items := make([]any, len(n.Content))
		for i, child := range n.Content {
			items[i], err = f.value(child)
			if err != nil {
				return nil, err
			}
		}
		value = items
	case yaml.AliasNode:
		value, err = f.value(n.Alias)
	default:
		value, err = frontScalarValue(n)
	}
	if err != nil {
		return nil, err
	}
	f.values[n] = value
	return value, nil
}

func (f *frontDecoder) mapping(n *yaml.Node) (map[string]any, error) {
	props := make(map[string]any, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if _, exists := props[key]; exists {
			return nil, fmt.Errorf("repeated key %q", key)
		}
		value, err := f.value(n.Content[i+1])
		if err != nil {
			return nil, err
		}
		props[key] = value
	}
	return props, nil
}

func frontScalarValue(n *yaml.Node) (any, error) {
	switch n.Tag {
	case "!!null":
		return nil, nil
	case "!!bool":
		var value bool
		err := n.Decode(&value)
		return value, err
	case "!!int":
		var value int64
		err := n.Decode(&value)
		return value, err
	case "!!float":
		var value float64
		if err := n.Decode(&value); err != nil {
			return nil, err
		}
		if !math.IsInf(value, 0) && !math.IsNaN(value) {
			return value, nil
		}
	}
	return n.Value, nil
}

// Only immediate, non-null scalars contribute tags, aliases or links.
func frontScalars(n *yaml.Node) []*yaml.Node {
	items := []*yaml.Node{n}
	if n.Kind == yaml.SequenceNode {
		items = n.Content
	}
	var scalars []*yaml.Node
	for _, item := range items {
		if item.Kind == yaml.ScalarNode && item.Tag != "!!null" {
			scalars = append(scalars, item)
		}
	}
	return scalars
}

func collectFrontTags(d *Doc, n *yaml.Node, line int) {
	for _, item := range frontScalars(n) {
		names := []string{item.Value}
		if n.Kind == yaml.ScalarNode {
			names = strings.FieldsFunc(item.Value, func(r rune) bool {
				return r == ',' || unicode.IsSpace(r)
			})
		}
		for _, name := range names {
			if name = strings.TrimPrefix(name, "#"); name != "" {
				d.Tags = append(d.Tags, Tag{Name: name, Front: true, Line: line})
			}
		}
	}
}

func collectPropertyLinks(d *Doc, s *source, n *yaml.Node, key string, line, frontLine int) {
	decoded := wikilinkPattern.FindAllStringSubmatchIndex(n.Value, -1)
	if len(decoded) == 0 {
		return
	}
	start := frontNodeOffset(s, n, frontLine)
	raw := wikilinkPattern.FindAllIndex(s.note[start:], len(decoded))
	for i, match := range decoded {
		target, subpath, alias, ok := splitWikilink(n.Value[match[2]:match[3]])
		if !ok || i >= len(raw) {
			continue
		}
		d.Links = append(d.Links, Link{
			Kind: LinkProperty, Target: target, Subpath: subpath, Alias: alias,
			Embed: n.Value[match[0]] == '!', Key: key, Line: line,
			Start: start + raw[i][0], End: start + raw[i][1],
		})
	}
}

// YAML columns count runes; source positions count bytes in the whole note.
func frontNodeOffset(s *source, n *yaml.Node, frontLine int) int {
	line := frontLine + n.Line - 1
	offset := s.newlines[line-2] + 1
	for column := 1; column < n.Column && offset < len(s.note); column++ {
		_, size := utf8.DecodeRune(s.note[offset:])
		offset += size
	}
	return offset
}
