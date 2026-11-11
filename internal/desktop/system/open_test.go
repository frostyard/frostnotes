package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebURLs(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://example.com/a?b=c", "https://example.com/a?b=c"},
		{"http://example.com", "http://example.com"},
		{"mailto:a@example.com?subject=hi&attach=/etc/passwd&body=x", "mailto:a@example.com?subject=hi&body=x"},
		{"mailto:a@e.test?attach=/etc/passwd&subject=Hi%20there&Attachment=x&cc=b@e.test&ATTACH=y&body=a+b", "mailto:a@e.test?subject=Hi%20there&cc=b@e.test&body=a+b"},
		{"mailto:a@e.test?attach=/home/u/.ssh/id_ed25519", "mailto:a@e.test"},
		{"mailto:a@e.test?%61ttach=x&To=c@e.test", "mailto:a@e.test?To=c@e.test"},
		{"mailto:a@e.test", "mailto:a@e.test"},
	} {
		got, err := WebURL(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("WebURL(%q) = %q, %v", tc.in, got, err)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "http://", "smb://host/x", "no scheme"} {
		if got, err := WebURL(bad); err == nil {
			t.Errorf("WebURL(%q) = %q", bad, got)
		}
	}
}

func TestVaultFileRules(t *testing.T) {
	root := t.TempDir()
	write := func(name string, mode os.FileMode) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("a/doc.pdf", 0o644)
	write("run.pdf", 0o755)
	write("page.html", 0o644)
	write("app.desktop", 0o644)
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.pdf")); err != nil {
		t.Fatal(err)
	}
	if got, err := VaultFile(root, "a/doc.pdf"); err != nil || !strings.HasSuffix(got, "a/doc.pdf") {
		t.Fatalf("a document: %q, %v", got, err)
	}
	for _, bad := range []string{"", "../secret.pdf", "/etc/passwd", "a/../a/doc.pdf", "a", "run.pdf", "page.html", "app.desktop", "link.pdf", "missing.pdf"} {
		if got, err := VaultFile(root, bad); err == nil {
			t.Errorf("VaultFile(%q) = %q", bad, got)
		}
	}
	if _, err := VaultFile("relative", "a/doc.pdf"); err == nil {
		t.Error("a relative vault folder was accepted")
	}
}
