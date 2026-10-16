package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const validCard = `---
id: "0007"
title: Normalize subjects
milestone: M0
size: S
touch: [internal/mime/subject.go]
given: [internal/mime/subject_test.go]
acceptance: go test ./internal/mime -run TestNormalizeSubject -count=1
---
# T-0007: Normalize subjects

Body text.
`

func TestParseCard(t *testing.T) {
	c, err := ParseCard([]byte(validCard))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "0007" || c.Size != "S" || !reflect.DeepEqual(c.Touch, []string{"internal/mime/subject.go"}) {
		t.Fatalf("card = %+v", c)
	}
	if !strings.HasPrefix(c.Body, "# T-0007") {
		t.Fatalf("body = %q", c.Body)
	}
}

func TestParseCardRejects(t *testing.T) {
	for name, tc := range map[string]struct{ edit, want string }{
		"no front matter": {"---\n", "front matter"},
		"bad id":          {`id: "0007"`, "four digits"},
		"escaping path":   {"touch: [internal/mime/subject.go]", "repository-relative"},
		"given touched":   {"touch: [internal/mime/subject.go]", "both given and touched"},
		"unknown key":     {"size: S", "not found"},
		"bad size":        {"size: S", "must be S, M or L"},
	} {
		t.Run(name, func(t *testing.T) {
			var src string
			switch name {
			case "no front matter":
				src = strings.TrimPrefix(validCard, "---\n")
			case "bad id":
				src = strings.Replace(validCard, tc.edit, `id: "7"`, 1)
			case "escaping path":
				src = strings.Replace(validCard, tc.edit, "touch: [../outside.go]", 1)
			case "given touched":
				src = strings.Replace(validCard, tc.edit, "touch: [internal/mime/subject_test.go]", 1)
			case "unknown key":
				src = strings.Replace(validCard, tc.edit, "size: S\nowner: me", 1)
			case "bad size":
				src = strings.Replace(validCard, tc.edit, "size: XL", 1)
			}
			if _, err := ParseCard([]byte(src)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseCard error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestScopeViolations(t *testing.T) {
	c, err := ParseCard([]byte(validCard))
	if err != nil {
		t.Fatal(err)
	}
	got := ScopeViolations(c, []string{
		"internal/mime/subject.go", "docs/tasks/doing/0007-subjects.md", "go.mod", "internal/mime/other.go",
	})
	if !reflect.DeepEqual(got, []string{"go.mod", "internal/mime/other.go"}) {
		t.Fatalf("violations = %v", got)
	}
}

func TestFindAndListCards(t *testing.T) {
	root := t.TempDir()
	for state, file := range map[string]string{"todo": "0007-subjects.md", "done": "0001-hello.md"} {
		dir := filepath.Join(root, "docs/tasks", state)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := validCard
		if state == "done" {
			body = strings.Replace(validCard, `id: "0007"`, `id: "0001"`, 1)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := FindCard(root, "0007")
	if err != nil || c.State != "todo" || c.Path != "docs/tasks/todo/0007-subjects.md" {
		t.Fatalf("FindCard = %+v, %v", c, err)
	}
	if c.GivenSource("internal/mime/subject_test.go") != "docs/tasks/todo/0007-subjects/_given/internal/mime/subject_test.go" {
		t.Fatalf("GivenSource = %s", c.GivenSource("internal/mime/subject_test.go"))
	}
	cards, err := ListCards(root)
	if err != nil || len(cards) != 2 || cards[0].ID != "0007" || cards[1].State != "done" {
		t.Fatalf("ListCards = %v, %v", cards, err)
	}
}

func TestTouchesApp(t *testing.T) {
	c, err := ParseCard([]byte(validCard))
	if err != nil {
		t.Fatal(err)
	}
	if TouchesApp(c) {
		t.Errorf("a Go-only card touches the app")
	}
	c.Touch = append(c.Touch, "app/src/lib/format.ts")
	if !TouchesApp(c) {
		t.Errorf("a card with app/src/lib/format.ts does not touch the app")
	}
}
