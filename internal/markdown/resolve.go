package markdown

import (
	"cmp"
	"path"
	"slices"
	"strings"
)

// Resolve finds the file a link names, by Obsidian's rules
// (docs/specs/markdown.md#link-resolution). source is the linking note's
// vault path and target the link's Target. files returns every vault path
// whose last segment, lower-cased, equals name. Resolve returns the path
// as files spells it, or "" when the link is unresolved.
func Resolve(source, target string, files func(name string) []string) string {
	if target == "" {
		return source
	}
	rooted := strings.HasPrefix(target, "/")
	target = strings.TrimLeft(target, "/")
	names := []string{target}
	if !strings.HasSuffix(strings.ToLower(target), ".md") {
		names = []string{target + ".md", target}
	}
	folder := path.Dir(source)
	for i, name := range names {
		names[i] = path.Clean(name)
		wanted := names[i]
		if !rooted {
			wanted = path.Join(folder, wanted)
		}
		if match := resolveName(wanted, folder, false, files); match != "" {
			return match
		}
	}
	if rooted || strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") {
		return ""
	}
	for _, name := range names {
		if match := resolveName(name, folder, true, files); match != "" {
			return match
		}
	}
	return ""
}

func resolveName(name, folder string, suffix bool, files func(string) []string) string {
	if name == ".." || strings.HasPrefix(name, "../") {
		return ""
	}
	lower := strings.ToLower(name)
	var matches []string
	for _, file := range files(strings.ToLower(path.Base(name))) {
		if matchesResolvePath(strings.ToLower(file), lower, suffix) {
			matches = append(matches, file)
		}
	}
	if len(matches) == 0 {
		return ""
	}
	slices.SortFunc(matches, func(a, b string) int {
		return compareResolvePaths(a, b, name, folder, suffix)
	})
	return matches[0]
}

func matchesResolvePath(file, name string, suffix bool) bool {
	return file == name || (suffix && strings.HasSuffix(file, "/"+name))
}

func compareResolvePaths(a, b, name, folder string, suffix bool) int {
	if suffix {
		aLocal, bLocal := path.Dir(a) == folder, path.Dir(b) == folder
		if aLocal != bLocal {
			if aLocal {
				return -1
			}
			return 1
		}
	}
	aExact := matchesResolvePath(a, name, suffix)
	bExact := matchesResolvePath(b, name, suffix)
	if aExact != bExact {
		if aExact {
			return -1
		}
		return 1
	}
	if suffix {
		if depth := cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")); depth != 0 {
			return depth
		}
	}
	return strings.Compare(a, b)
}
