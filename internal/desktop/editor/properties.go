package editor

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// A note's properties as the YAML text of its frontmatter
// (docs/design/app.md#properties), edited by key without rewriting
// untouched text. It ports app/src/editor/properties.ts.

// Property is a key of the frontmatter and its value: a string, int64,
// float64, bool, nil, []any or map[string]any.
type Property struct {
	Key   string
	Value any
}

// Frontmatter is what a note's frontmatter holds.
type Frontmatter struct {
	Exists     bool
	Properties []Property
	Error      string // why it is not a mapping, "" when it is
}

// frontSpan finds frontmatter at the start of text: the opening line's
// ending, what is between it and the first closing line ("---" or "..."),
// and where that line ends.
func frontSpan(text string) (ending, body string, end int, ok bool) {
	first, rest, found := strings.Cut(text, "\n")
	if !found || strings.TrimSuffix(first, "\r") != "---" {
		return "", "", 0, false
	}
	ending = text[len("---") : len(first)+1]
	start := len(first) + 1
	for at := start; at <= len(text); {
		next := len(text)
		line := rest[at-start:]
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line, next = line[:i], at+i+1
		}
		if t := strings.TrimSuffix(line, "\r"); t == "---" || t == "..." {
			return ending, text[start:at], next, true
		}
		if next == len(text) {
			break
		}
		at = next
	}
	return "", "", 0, false
}

type frontText struct {
	exists bool
	start  int
	text   string
	ending string
}

func splitFront(text string) frontText {
	bom := 0
	if strings.HasPrefix(text, "\uFEFF") {
		bom = len("\uFEFF")
	}
	ending, body, _, ok := frontSpan(text[bom:])
	if !ok {
		return frontText{start: bom, ending: "\n"}
	}
	return frontText{exists: true, start: bom + 3 + len(ending), text: body, ending: ending}
}

type unit struct{ start, end int }

type parsedFront struct {
	props []Property
	lines []string
	units map[string]unit
}

// maxExpanded bounds the nodes a note's frontmatter expands to through YAML
// aliases, all its properties together: a note can name itself, double at
// each level, or name one large value many times. What is written out costs
// nothing, as the file's size bounds it.
const maxExpanded = 10000

// nodeValue is a node's value, its aliases followed. An alias inside what
// it names, or past the bound left, is nil.
func nodeValue(n *yaml.Node, left *int) any {
	return expand(n, map[*yaml.Node]bool{}, left)
}

func expand(n *yaml.Node, open map[*yaml.Node]bool, left *int) any {
	if len(open) > 0 { // inside an alias
		if *left--; *left < 0 {
			return nil
		}
	}
	switch n.Kind {
	case yaml.AliasNode:
		if open[n.Alias] {
			return nil
		}
		open[n.Alias] = true
		defer delete(open, n.Alias)
		return expand(n.Alias, open, left)
	case yaml.SequenceNode:
		out := make([]any, len(n.Content))
		for i, c := range n.Content {
			out[i] = expand(c, open, left)
		}
		return out
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			out[n.Content[i].Value] = expand(n.Content[i+1], open, left)
		}
		return out
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!null":
			return nil
		case "!!bool":
			var b bool
			_ = n.Decode(&b)
			return b
		case "!!int":
			var v int64
			if n.Decode(&v) == nil {
				return v
			}
		case "!!float":
			var v float64
			if n.Decode(&v) == nil {
				return v
			}
		}
		return n.Value // strings and dates, as written
	}
	return nil
}

func parseFront(text string) (*parsedFront, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	p := &parsedFront{lines: strings.SplitAfter(text, "\n"), units: map[string]unit{}}
	if n := len(p.lines); n > 0 && p.lines[n-1] == "" {
		p.lines = p.lines[:n-1]
	}
	if doc.Kind == 0 {
		return p, nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.ShortTag() == "!!null" {
		return p, nil
	}
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("frontmatter must be a block YAML mapping")
	}
	seen := map[string]bool{}
	left := maxExpanded
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		if seen[key.Value] {
			return nil, fmt.Errorf("the key %s appears twice", key.Value)
		}
		seen[key.Value] = true
		p.props = append(p.props, Property{Key: key.Value, Value: nodeValue(val, &left)})
		at := key.Line - 1
		if at < 0 || at >= len(p.lines) || strings.ContainsAny(p.lines[at][:min(1, len(p.lines[at]))], " \t#-") {
			continue
		}
		p.units[key.Value] = unit{at, unitEnd(p.lines, at+1)}
	}
	return p, nil
}

// indented reports whether a line continues a property's value: indented,
// or a list item under its key. A key may itself start with "-".
func indented(line string) bool {
	return line != "" && strings.ContainsAny(line[:1], " \t") || line == "-" || strings.HasPrefix(line, "- ")
}

// unitEnd is where a property's lines end, keeping trailing blank lines
// unless its value resumes after them.
func unitEnd(lines []string, start int) int {
	for start < len(lines) {
		line := lines[start]
		switch {
		case strings.TrimSpace(line) == "":
			next := start + 1
			for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
				next++
			}
			if next >= len(lines) || !indented(lines[next]) {
				return start
			}
			start = next
		case indented(line):
			start++
		default:
			return start
		}
	}
	return start
}

func valueNode(v any) *yaml.Node {
	switch x := v.(type) {
	case string:
		return stringNode(x)
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, c := range x {
			n.Content = append(n.Content, valueNode(c))
		}
		return n
	case []string:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, c := range x {
			n.Content = append(n.Content, stringNode(c))
		}
		return n
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			n.Content = append(n.Content, stringNode(k), valueNode(x[k]))
		}
		return n
	}
	n := &yaml.Node{}
	if err := n.Encode(v); err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}
	}
	return n
}

var (
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$`)
)

// stringNode writes a date plain, as Obsidian does, and quotes in double
// quotes what YAML cannot hold plain, as a [[link]].
func stringNode(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	switch {
	case dateRe.MatchString(s):
		n.Tag = "!!timestamp"
	case s != "" && strings.ContainsAny(s[:1], "[]{}*&!|>'\"%@`#,?:-") || strings.Contains(s, ": ") || strings.Contains(s, " #"):
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

func renderProperty(key string, value any, ending string) (string, error) {
	doc := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{stringNode(key), valueNode(value)}}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.ReplaceAll(b.String(), "\n", ending), nil
}

func replaceUnit(lines []string, u unit, rendered string) {
	lines[u.start] = rendered
	for i := u.start + 1; i < u.end; i++ {
		lines[i] = ""
	}
}

func replaceFront(text string, f frontText, replacement string) (string, error) {
	if !f.exists && replacement == "" {
		return text, nil
	}
	var result string
	if f.exists {
		result = text[:f.start] + replacement + text[f.start+len(f.text):]
	} else {
		result = text[:f.start] + "---\n" + replacement + "---\n" + text[f.start:]
	}
	checked := ReadFrontmatter(result)
	if checked.Error != "" {
		return "", errors.New(checked.Error)
	}
	if !checked.Exists {
		return "", errors.New("the result has no frontmatter mapping")
	}
	return result, nil
}

// ReadFrontmatter reads a note's properties in order.
func ReadFrontmatter(text string) Frontmatter {
	f := splitFront(text)
	if !f.exists {
		return Frontmatter{}
	}
	p, err := parseFront(f.text)
	if err != nil {
		return Frontmatter{Exists: true, Error: err.Error()}
	}
	return Frontmatter{Exists: true, Properties: p.props}
}

// SetProperties returns text with each key of props set to its value, a
// key whose value is nil removed.
func SetProperties(text string, props map[string]any) (string, error) {
	f := splitFront(text)
	p, err := parseFront(f.text)
	if err != nil {
		return "", err
	}
	added := ""
	for _, key := range slices.Sorted(maps.Keys(props)) {
		rendered := ""
		if props[key] != nil {
			if rendered, err = renderProperty(key, props[key], f.ending); err != nil {
				return "", err
			}
		}
		if u, ok := p.units[key]; ok {
			replaceUnit(p.lines, u, rendered)
		} else {
			added += rendered
		}
	}
	return replaceFront(text, f, strings.Join(p.lines, "")+added)
}

// RenameProperty returns text with key from renamed to, its value kept, in
// its place.
func RenameProperty(text, from, to string) (string, error) {
	if from == to {
		return text, nil
	}
	f := splitFront(text)
	p, err := parseFront(f.text)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(p.props, func(x Property) bool { return x.Key == from })
	if i < 0 {
		return "", fmt.Errorf("property %s does not exist", from)
	}
	if slices.ContainsFunc(p.props, func(x Property) bool { return x.Key == to }) {
		return "", fmt.Errorf("property %s already exists", to)
	}
	u, ok := p.units[from]
	if !ok {
		return "", fmt.Errorf("property %s has no editable lines", from)
	}
	rendered, err := renderProperty(to, p.props[i].Value, f.ending)
	if err != nil {
		return "", err
	}
	replaceUnit(p.lines, u, rendered)
	return replaceFront(text, f, strings.Join(p.lines, ""))
}

// The types of a property, as Obsidian names them.
const (
	TypeText      = "text"
	TypeMultitext = "multitext"
	TypeNumber    = "number"
	TypeCheckbox  = "checkbox"
	TypeDate      = "date"
	TypeDatetime  = "datetime"
	TypeAliases   = "aliases"
	TypeTags      = "tags"
)

// PropertyType is a property's type: the vault's type for its key, else
// what the key or value says.
func PropertyType(key string, value any, vaultType string) string {
	switch vaultType {
	case TypeText, TypeMultitext, TypeNumber, TypeCheckbox, TypeDate, TypeDatetime, TypeAliases, TypeTags:
		return vaultType
	}
	if name := strings.ToLower(key); name == TypeTags || name == TypeAliases {
		return name
	}
	switch v := value.(type) {
	case []any:
		return TypeMultitext
	case bool:
		return TypeCheckbox
	case int64, float64:
		return TypeNumber
	case string:
		if dateRe.MatchString(v) {
			return TypeDate
		}
		if dateTimeRe.MatchString(v) {
			return TypeDatetime
		}
	}
	return TypeText
}

// valueText is a value as a text field shows it.
func valueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// listItems is a list value's items as text; any other value is one item.
func listItems(v any) []string {
	if l, ok := v.([]any); ok {
		out := make([]string, len(l))
		for i, x := range l {
			out[i] = valueText(x)
		}
		return out
	}
	if t := valueText(v); t != "" {
		return []string{t}
	}
	return nil
}

// frontEnd is the offset just after the frontmatter, 0 without one.
func frontEnd(text string) int {
	bom := 0
	if strings.HasPrefix(text, "\uFEFF") {
		bom = len("\uFEFF")
	}
	_, _, end, ok := frontSpan(text[bom:])
	if !ok {
		return 0
	}
	return bom + end
}

// FrontmatterStart is where a note's frontmatter starts or would start:
// after a byte order mark.
func FrontmatterStart(text string) int {
	if f := splitFront(text); !f.exists {
		return f.start
	}
	return len(text) - len(strings.TrimPrefix(text, "\uFEFF"))
}

// Body is a note's text after its frontmatter.
func Body(text string) string { return text[frontEnd(text):] }

// FrontmatterLastLine is the line of the frontmatter's closing "---", from
// 0; it is 0 without frontmatter.
func FrontmatterLastLine(text string) int {
	end := frontEnd(text)
	if end == 0 {
		return 0
	}
	line := strings.Count(text[:end], "\n")
	if !strings.HasSuffix(text[:end], "\n") {
		return line
	}
	return line - 1
}
