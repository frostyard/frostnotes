package query

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Limits on a base's YAML.
const (
	maxYAMLDepth   = 200
	maxFilterDepth = 32
	maxArgs        = 20000 // SQL arguments of one query
)

// node is a YAML value of a base, checked for what the subset allows: no
// anchors or aliases, scalar keys that do not repeat.
type node struct {
	kind  yaml.Kind
	tag   string // the resolved short tag, such as !!str
	value string
	pairs []pair  // a mapping's, in order
	items []*node // a sequence's
}

type pair struct {
	key string
	val *node
}

func (n *node) isString() bool   { return n != nil && n.kind == yaml.ScalarNode && n.tag == "!!str" }
func (n *node) isNull() bool     { return n == nil || n.kind == yaml.ScalarNode && n.tag == "!!null" }
func (n *node) isMapping() bool  { return n != nil && n.kind == yaml.MappingNode }
func (n *node) isSequence() bool { return n != nil && n.kind == yaml.SequenceNode }

// get returns the value of key in a mapping, or nil.
func (n *node) get(key string) *node {
	if !n.isMapping() {
		return nil
	}
	for _, p := range n.pairs {
		if p.key == key {
			return p.val
		}
	}
	return nil
}

// parseYAML reads source's one YAML document. Empty text is an empty
// mapping.
func parseYAML(source []byte) (*node, *Error) {
	dec := yaml.NewDecoder(bytes.NewReader(source))
	var doc yaml.Node
	if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
		return &node{kind: yaml.MappingNode}, nil
	} else if err != nil {
		return nil, &Error{Location: "base", Msg: fmt.Sprintf("not valid YAML: %v", err)}
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, &Error{Location: "base", Msg: "more than one YAML document"}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, &Error{Location: "base", Msg: "not valid YAML"}
	}
	root, err := convert(doc.Content[0], "", 0)
	if err != nil {
		return nil, err
	}
	if root.isNull() {
		return &node{kind: yaml.MappingNode}, nil
	}
	if !root.isMapping() {
		return nil, &Error{Location: "base", Msg: "a base is a YAML mapping"}
	}
	return root, nil
}

func convert(n *yaml.Node, loc string, depth int) (*node, *Error) {
	if depth > maxYAMLDepth {
		return nil, &Error{Location: loc, Msg: fmt.Sprintf("the YAML nests more than %d levels", maxYAMLDepth)}
	}
	out := &node{kind: n.Kind, tag: n.ShortTag(), value: n.Value}
	switch n.Kind {
	case yaml.AliasNode:
		return nil, &Error{Location: loc, Msg: "YAML anchors and aliases are outside the subset"}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			item, err := convert(c, fmt.Sprintf("%s[%d]", loc, i), depth+1)
			if err != nil {
				return nil, err
			}
			out.items = append(out.items, item)
		}
	case yaml.MappingNode:
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, &Error{Location: loc, Msg: "a mapping key is not a string"}
			}
			here := k.Value
			if loc != "" {
				here = loc + "." + k.Value
			}
			child := n.Content[i+1]
			if !keepKey(loc, depth, k.Value) {
				// Outside the subset: ignored, so its YAML is not checked.
				out.pairs = append(out.pairs, pair{k.Value, &node{kind: child.Kind, tag: child.ShortTag()}})
				continue
			}
			if seen[k.Value] {
				return nil, &Error{Location: here, Msg: "the key is given twice"}
			}
			seen[k.Value] = true
			val, err := convert(child, here, depth+1)
			if err != nil {
				return nil, err
			}
			out.pairs = append(out.pairs, pair{k.Value, val})
		}
	}
	return out, nil
}

// keepKey says whether a mapping's key is read: the top-level keys of the
// subset and a view's keys; every other key there is ignored.
func keepKey(loc string, depth int, key string) bool {
	switch {
	case depth == 0:
		return key == "filters" || key == "properties" || key == "views"
	case depth == 2 && strings.HasPrefix(loc, "views["):
		switch key {
		case "type", "name", "filters", "order", "sort", "limit", "groupBy", "groupOrder", "image":
			return true
		}
		return false
	}
	return true
}
