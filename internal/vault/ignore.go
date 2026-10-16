package vault

import (
	"fmt"
	"path"
	"strings"
)

// Ignore holds validated, case-sensitive vault ignore patterns.
type Ignore struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	segments []string
	anchored bool
	dirOnly  bool
}

// NewIgnore validates patterns in the supported subset of gitignore syntax.
func NewIgnore(patterns []string) (*Ignore, error) {
	ig := &Ignore{}
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "!") {
			return nil, fmt.Errorf("ignore pattern %q: negation is not supported", raw)
		}
		dirOnly := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		anchored := strings.Contains(pattern, "/")
		pattern = strings.TrimPrefix(pattern, "/")
		if pattern == "" {
			return nil, fmt.Errorf("ignore pattern %q: empty pattern", raw)
		}
		segments := strings.Split(pattern, "/")
		for _, segment := range segments {
			if segment == "" {
				return nil, fmt.Errorf("ignore pattern %q: empty segment", raw)
			}
			if _, err := path.Match(segment, ""); err != nil {
				return nil, fmt.Errorf("ignore pattern %q: %w", raw, err)
			}
		}
		ig.patterns = append(ig.patterns, ignorePattern{segments: segments, anchored: anchored, dirOnly: dirOnly})
	}
	return ig, nil
}

// Match reports whether a path or any of its parent folders is ignored.
// A nil *Ignore matches nothing; dir indicates whether the path itself is a folder.
func (ig *Ignore) Match(name string, dir bool) bool {
	if ig == nil {
		return false
	}
	segments := strings.Split(name, "/")
	for _, pattern := range ig.patterns {
		for end := 1; end <= len(segments); end++ {
			if pattern.dirOnly && end == len(segments) && !dir {
				continue
			}
			candidate := segments[:end]
			if !pattern.anchored {
				candidate = segments[end-1 : end]
			}
			if matchSegments(pattern.segments, candidate) {
				return true
			}
		}
	}
	return false
}

func matchSegments(pattern, segments []string) bool {
	// matched[i] means the pattern processed so far matches the first i segments.
	// Keeping a row of states avoids exponential backtracking across multiple **s.
	matched := make([]bool, len(segments)+1)
	matched[0] = true
	for _, segment := range pattern {
		if segment == "**" {
			for i := 1; i <= len(segments); i++ {
				matched[i] = matched[i] || matched[i-1]
			}
			continue
		}
		for i := len(segments); i > 0; i-- {
			ok, _ := path.Match(segment, segments[i-1]) // Validated by NewIgnore.
			matched[i] = matched[i-1] && ok
		}
		matched[0] = false
	}
	return matched[len(segments)]
}
