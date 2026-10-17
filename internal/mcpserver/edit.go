package mcpserver

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/internal/markdown"
	"go.yaml.in/yaml/v3"
)

func applyEdit(content string, in editIn) (string, error) {
	count := 0
	for _, given := range []bool{in.OldText != "", in.Append != "", in.Prepend != "", in.Properties != nil} {
		if given {
			count++
		}
	}
	if count != 1 {
		return "", fmt.Errorf("exactly one edit is required")
	}
	if in.NewText != "" && in.OldText == "" {
		return "", fmt.Errorf("new_text requires old_text")
	}
	switch {
	case in.OldText != "":
		return replaceOnce(content, in.OldText, in.NewText)
	case in.Append != "":
		return appendText(content, in.Append), nil
	case in.Prepend != "":
		return prependText(content, in.Prepend), nil
	default:
		if len(in.Properties) == 0 {
			return "", fmt.Errorf("properties must not be empty")
		}
		return setProperties(content, in.Properties)
	}
}

func replaceOnce(content, old, replacement string) (string, error) {
	if old == "" {
		return "", fmt.Errorf("old_text is empty")
	}
	count := strings.Count(content, old)
	switch count {
	case 0:
		return "", fmt.Errorf("old_text does not occur")
	case 1:
		return strings.Replace(content, old, replacement, 1), nil
	default:
		return "", fmt.Errorf("old_text occurs %d times", count)
	}
}

func appendText(content, text string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return content + text
}

func prependText(content, text string) string {
	split := markdown.SplitFrontmatter([]byte(content))
	start := split.BodyStart
	if !split.HasFront {
		start = leadingBOM(content)
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return content[:start] + text + content[start:]
}

func leadingBOM(content string) int {
	if strings.HasPrefix(content, "\ufeff") {
		return len("\ufeff")
	}
	return 0
}

func setProperties(content string, props map[string]any) (string, error) {
	if err := markdown.Parse([]byte(content)).PropError; err != nil {
		return "", fmt.Errorf("set properties: %w", err)
	}
	split := markdown.SplitFrontmatter([]byte(content))
	start, ending := leadingBOM(content), "\n"
	if split.HasFront && strings.HasPrefix(content[start:], "---\r\n") {
		ending = "\r\n"
	}
	front, err := editProperties(string(split.Front), props, ending)
	if err != nil {
		return "", err
	}
	var result string
	if split.HasFront {
		start += len("---") + len(ending)
		result = content[:start] + front + content[start+len(split.Front):]
	} else {
		if front == "" {
			return content, nil
		}
		result = content[:start] + "---\n" + front + "---\n" + content[start:]
	}
	parsed := markdown.Parse([]byte(result))
	if parsed.PropError != nil {
		return "", fmt.Errorf("set properties result: %w", parsed.PropError)
	}
	if parsed.Props == nil {
		return "", fmt.Errorf("set properties result has no frontmatter mapping")
	}
	return result, nil
}

func editProperties(front string, props map[string]any, ending string) (string, error) {
	lines := strings.SplitAfter(front, "\n")
	units, err := propertyUnits(front, lines)
	if err != nil {
		return "", err
	}
	var added strings.Builder
	for _, key := range slices.Sorted(maps.Keys(props)) {
		rendered, err := renderProperty(key, props[key], ending)
		if err != nil {
			return "", err
		}
		if unit, ok := units[key]; ok {
			lines[unit.start] = rendered
			for i := unit.start + 1; i < unit.end; i++ {
				lines[i] = ""
			}
		} else {
			added.WriteString(rendered)
		}
	}
	return strings.Join(lines, "") + added.String(), nil
}

type propertyUnit struct {
	start, end int // line indexes, with end exclusive
}

// YAML supplies decoded key names and positions, never replacement text for
// untouched keys. Find all ranges before editing so removals cannot move them.
func propertyUnits(front string, lines []string) (map[string]propertyUnit, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(front), &document); err != nil {
		return nil, fmt.Errorf("read property lines: %w", err)
	}
	units := make(map[string]propertyUnit)
	if len(document.Content) == 0 {
		return units, nil
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		start := key.Line - 1
		line := lines[start]
		if line == "" || strings.ContainsRune(" \t#-", rune(line[0])) {
			continue
		}
		units[key.Value] = propertyUnit{start: start, end: propertyUnitEnd(lines, start+1)}
	}
	return units, nil
}

func propertyUnitEnd(lines []string, start int) int {
	for start < len(lines) {
		line := lines[start]
		if strings.TrimSpace(line) == "" {
			// Blank lines belong to a value only when indentation resumes.
			next := start + 1
			for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
				next++
			}
			if next == len(lines) || !strings.ContainsRune(" \t", rune(lines[next][0])) {
				break
			}
			start = next
		} else if strings.ContainsRune(" \t-", rune(line[0])) {
			start++
		} else {
			break
		}
	}
	return start
}

func renderProperty(key string, value any, ending string) (string, error) {
	if value == nil {
		return "", nil
	}
	var out strings.Builder
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(map[string]any{key: value}); err != nil {
		return "", fmt.Errorf("render property %q: %w", key, err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("finish property %q: %w", key, err)
	}
	return strings.ReplaceAll(out.String(), "\n", ending), nil
}
