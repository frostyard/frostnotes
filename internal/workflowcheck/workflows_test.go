package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

var (
	commitAction = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	dockerAction = regexp.MustCompile(`^docker://[^@]+@sha256:[0-9a-f]{64}$`)
)

// immutable reports whether an action reference names one version for
// good: a local action, a commit, or an image digest.
func immutable(ref string) bool {
	return strings.HasPrefix(ref, "./") || commitAction.MatchString(ref) || dockerAction.MatchString(ref)
}

func TestPinnedReferencesAreImmutable(t *testing.T) {
	sha, digest := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for ref, want := range map[string]bool{
		"actions/checkout@" + sha:                   true,
		"owner/repo/.github/actions/publish@" + sha: true,
		"docker://alpine@sha256:" + digest:          true,
		"./.github/actions/check":                   true,
		"actions/checkout@v7":                       false,
		"owner/repo/.github/actions/publish@main":   false,
		"actions/checkout@deadbeef":                 false,
		"actions/checkout@${{inputs.ref}}":          false,
		"docker://alpine:3.22":                      false,
		"actions/checkout@" + strings.ToUpper(sha):  false,
	} {
		if got := immutable(ref); got != want {
			t.Errorf("immutable(%q) = %v, want %v", ref, got, want)
		}
	}
}

// workflows parses every workflow in .github/workflows.
func workflows(t *testing.T) map[string]*yaml.Node {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no workflows found")
	}
	out := map[string]*yaml.Node{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
			t.Fatalf("parse %s: %v", p, err)
		}
		out[filepath.Base(p)] = doc.Content[0]
	}
	return out
}

// get returns the value of key in a mapping node, or nil.
func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// walk calls f for every mapping node under n.
func walk(n *yaml.Node, f func(*yaml.Node)) {
	if n.Kind == yaml.MappingNode {
		f(n)
	}
	for _, c := range n.Content {
		walk(c, f)
	}
}

// Every external action is pinned to a full commit SHA (the version a
// trailing comment names is for people; the SHA is what runs), every
// workflow starts from declared permissions, and every checkout drops the
// token it would otherwise leave in .git/config.
func TestWorkflowsFollowADR0021(t *testing.T) {
	uses := 0
	for name, root := range workflows(t) {
		if get(root, "permissions") == nil {
			t.Errorf("%s: no top-level permissions (start from permissions: {})", name)
		}
		walk(root, func(m *yaml.Node) {
			ref := get(m, "uses")
			if ref == nil {
				return
			}
			uses++
			if ref.Kind != yaml.ScalarNode || !immutable(ref.Value) {
				t.Errorf("%s:%d: %q must name a full commit SHA or image digest", name, ref.Line, ref.Value)
			}
			if strings.HasPrefix(ref.Value, "actions/checkout@") {
				if pc := get(get(m, "with"), "persist-credentials"); pc == nil || pc.Value != "false" {
					t.Errorf("%s:%d: checkout must set persist-credentials: false", name, ref.Line)
				}
			}
		})
	}
	if uses == 0 {
		t.Fatal("no action references found")
	}
}

// The release workflow's build job runs third-party npm code, so it holds
// no write token and no id-token; only the release job may write, attest
// and mint an id-token, and the tag filter and tag check stay in place.
func TestReleaseWorkflowSplitsBuildFromRelease(t *testing.T) {
	root, ok := workflows(t)["release.yml"]
	if !ok {
		t.Fatal("no release.yml")
	}
	jobs := get(root, "jobs")
	for _, name := range []string{"build", "release"} {
		if get(jobs, name) == nil {
			t.Fatalf("release.yml has no %s job", name)
		}
	}
	perms := func(job string) map[string]string {
		out := map[string]string{}
		p := get(get(jobs, job), "permissions")
		if p == nil {
			t.Fatalf("%s job declares no permissions", job)
		}
		for i := 0; i+1 < len(p.Content); i += 2 {
			out[p.Content[i].Value] = p.Content[i+1].Value
		}
		return out
	}
	if b := perms("build"); len(b) != 1 || b["contents"] != "read" {
		t.Errorf("build permissions = %v, want only contents: read", b)
	}
	r := perms("release")
	if r["contents"] != "write" || r["id-token"] != "write" || r["attestations"] != "write" {
		t.Errorf("release permissions = %v, want contents, id-token and attestations write", r)
	}
	if n := get(get(root, "on"), "push"); n == nil || get(n, "tags") == nil {
		t.Error("release.yml must run on pushed tags")
	}
}

// CI gains the snapshot .deb job without renaming another (the
// required-checks ruleset matches job names exactly).
func TestCIHasTheSnapshotDebJob(t *testing.T) {
	root := workflows(t)["ci.yml"]
	job := get(get(root, "jobs"), "deb")
	if job == nil {
		t.Fatal("ci.yml has no deb job")
	}
	if n := get(job, "name"); n == nil || n.Value != "Snapshot .deb (make deb)" {
		t.Errorf("deb job name = %v", n)
	}
}
