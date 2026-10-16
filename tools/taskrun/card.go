package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// States are the docs/tasks subdirectories a card moves through.
var States = []string{"todo", "doing", "done"}

// Card is a task card: docs/tasks/<state>/<id>-<slug>.md with YAML front
// matter, plus an optional <id>-<slug>/_given/ tree of files (usually
// contract tests) copied into the repository when the task starts. The
// leading underscore keeps go ./... patterns out of it.
type Card struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Milestone  string   `yaml:"milestone"`
	Size       string   `yaml:"size"`
	Touch      []string `yaml:"touch"`
	Given      []string `yaml:"given"`
	Acceptance string   `yaml:"acceptance"`

	State string `yaml:"-"`
	Path  string `yaml:"-"` // the .md file, relative to the repo root
	Body  string `yaml:"-"` // the markdown after the front matter
}

var idRE = regexp.MustCompile(`^[0-9]{4}$`)

// Dir is the card's companion directory.
func (c *Card) Dir() string { return strings.TrimSuffix(c.Path, ".md") }

// GivenSource is where a given file is kept before the task starts.
func (c *Card) GivenSource(repoPath string) string {
	return path.Join(c.Dir(), "_given", repoPath) // a repository path, slash-separated
}

// ParseCard reads front matter and body.
func ParseCard(data []byte) (*Card, error) {
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return nil, errors.New("card must start with --- front matter")
	}
	front, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return nil, errors.New("card front matter is not closed with ---")
	}
	dec := yaml.NewDecoder(bytes.NewReader(front))
	dec.KnownFields(true)
	var c Card
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("card front matter: %w", err)
	}
	c.Body = string(body)
	return &c, c.validate()
}

func (c *Card) validate() error {
	var errs []error
	if !idRE.MatchString(c.ID) {
		errs = append(errs, fmt.Errorf("id %q must be four digits", c.ID))
	}
	if c.Title == "" || c.Acceptance == "" {
		errs = append(errs, errors.New("title and acceptance are required"))
	}
	if !slices.Contains([]string{"S", "M", "L"}, c.Size) {
		errs = append(errs, fmt.Errorf("size %q must be S, M or L (ADR-0010)", c.Size))
	}
	if len(c.Touch) == 0 {
		errs = append(errs, errors.New("touch must list at least one file"))
	}
	for _, p := range append(slices.Clone(c.Touch), c.Given...) {
		// Card paths are slash-separated on every OS.
		if path.IsAbs(p) || strings.Contains(p, "..") || path.Clean(p) != p {
			errs = append(errs, fmt.Errorf("path %q must be clean and repository-relative", p))
		}
	}
	for _, g := range c.Given {
		if slices.Contains(c.Touch, g) {
			errs = append(errs, fmt.Errorf("%s is both given and touched; given files are protected", g))
		}
	}
	return errors.Join(errs...)
}

// FindCard locates a card by ID in any state.
func FindCard(root, id string) (*Card, error) {
	for _, state := range States {
		matches, err := filepath.Glob(filepath.Join(root, "docs/tasks", state, id+"-*.md"))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			continue
		}
		data, err := os.ReadFile(matches[0])
		if err != nil {
			return nil, err
		}
		c, err := ParseCard(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", matches[0], err)
		}
		if c.ID != id {
			return nil, fmt.Errorf("%s: front matter id %s does not match the file name", matches[0], c.ID)
		}
		c.State = state
		c.Path, _ = filepath.Rel(root, matches[0])
		c.Path = filepath.ToSlash(c.Path) // repository paths are slash-separated
		return c, nil
	}
	return nil, fmt.Errorf("no card %s in docs/tasks/{todo,doing,done}", id)
}

// ListCards returns every card, by state then ID.
func ListCards(root string) ([]*Card, error) {
	var out []*Card
	for _, state := range States {
		matches, err := filepath.Glob(filepath.Join(root, "docs/tasks", state, "[0-9][0-9][0-9][0-9]-*.md"))
		if err != nil {
			return nil, err
		}
		slices.Sort(matches)
		for _, m := range matches {
			id := filepath.Base(m)[:4]
			c, err := FindCard(root, id)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// TouchesApp reports whether a card may change the app, whose gate is
// make ui-check rather than make check alone.
func TouchesApp(c *Card) bool {
	for _, p := range c.Touch {
		if strings.HasPrefix(p, "app/") {
			return true
		}
	}
	return false
}

// ScopeViolations lists changed paths a card may not change: anything outside
// Touch, except the card's own files under docs/tasks.
func ScopeViolations(c *Card, changed []string) []string {
	var bad []string
	for _, p := range changed {
		if slices.Contains(c.Touch, p) || strings.HasPrefix(p, "docs/tasks/") {
			continue
		}
		bad = append(bad, p)
	}
	return bad
}
