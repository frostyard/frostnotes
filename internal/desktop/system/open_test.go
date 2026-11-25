package system

import (
	"errors"
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
		// A fragment a mail client might read as more fields (#200).
		{"mailto:a@example.com?subject=x#&attach=/etc/passwd", "mailto:a@example.com?subject=x"},
		{"mailto:a@e.test#attach=/etc/passwd", "mailto:a@e.test"},
		{"mailto:a@e.test?subject&body=x", "mailto:a@e.test?body=x"},
		{"mailto:a@e.test?", "mailto:a@e.test"},
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

// mailtoCases are frostui's cases of SanitizeMailto (frostui v0.10.1,
// opener_test.go) with what it makes of them. While the MyGo client has
// frostui, a test of cmd/frostnotes also compares the two on these and
// more.
var mailtoCases = []struct{ name, in, want string }{
	{"attach between kept fields", "mailto:a@example.com?subject=hi&attach=/etc/passwd&body=x", "mailto:a@example.com?subject=hi&body=x"},
	{"attach, attachment and case", "mailto:a@e.test?attach=/etc/passwd&subject=Hi%20there&Attachment=x&cc=b@e.test&ATTACH=y&body=a+b", "mailto:a@e.test?subject=Hi%20there&cc=b@e.test&body=a+b"},
	{"attach alone", "mailto:a@e.test?attach=/home/u/.ssh/id_ed25519", "mailto:a@e.test"},
	{"encoded attach, kept To", "mailto:a@e.test?%61ttach=x&To=c@e.test", "mailto:a@e.test?To=c@e.test"},
	{"no fields", "mailto:a@e.test", "mailto:a@e.test"},

	{"each kept field", "mailto:a@e.test?to=b@e.test&cc=c@e.test&bcc=d@e.test&subject=s&body=b", "mailto:a@e.test?to=b@e.test&cc=c@e.test&bcc=d@e.test&subject=s&body=b"},
	{"kept fields in any case", "mailto:a@e.test?TO=b@e.test&Cc=c@e.test&bCC=d@e.test&SuBjEcT=s&BODY=b", "mailto:a@e.test?TO=b@e.test&Cc=c@e.test&bCC=d@e.test&SuBjEcT=s&BODY=b"},
	{"file and other headers", "mailto:a@e.test?file=/etc/passwd&in-reply-to=%3Cx@e.test%3E&references=r&from=f@e.test&reply-to=r@e.test&X-Mailer=m&sender=s@e.test", "mailto:a@e.test"},
	{"the scheme in capitals", "MAILTO:a@e.test?Subject=x&FILE=y", "mailto:a@e.test?Subject=x"},
	{"repeated fields, in order", "mailto:a@e.test?to=b@e.test&attach=x&to=c@e.test&subject=1&subject=2&body=1&body=2", "mailto:a@e.test?to=b@e.test&to=c@e.test&subject=1&subject=2&body=1&body=2"},
	{"repeated attach", "mailto:a@e.test?attach=x&attach=y&ATTACH=z", "mailto:a@e.test"},
	{"plus and %20 as written", "mailto:a@e.test?subject=a+b&body=c%20d+e", "mailto:a@e.test?subject=a+b&body=c%20d+e"},
	{"encoded kept names", "mailto:a@e.test?%74o=b@e.test&%53ubject=x&body=y", "mailto:a@e.test?body=y"},
	{"double encoded attach", "mailto:a@e.test?%2561ttach=x&subject=y", "mailto:a@e.test?subject=y"},
	{"Unicode folding", "mailto:a@e.test?ſubject=x&body=y", "mailto:a@e.test?body=y"},
	{"fields without =", "mailto:a@e.test?subject&body=x&attach&to", "mailto:a@e.test?body=x"},
	{"empty fields and values", "mailto:a@e.test?&&subject=&&body=x&", "mailto:a@e.test?subject=&body=x"},
	{"an address and to=", "mailto:a@e.test?to=b@e.test&attach=x", "mailto:a@e.test?to=b@e.test"},
	{"to= without an address", "mailto:?to=b@e.test&attach=x", "mailto:?to=b@e.test"},
	{"a fragment carrying a field", "mailto:a@e.test?subject=x#&attach=/etc/passwd", "mailto:a@e.test?subject=x"},
	{"a fragment without fields", "mailto:a@e.test#attach=x", "mailto:a@e.test"},
	{"malformed escapes", "mailto:a@e.test?subject=100%&body=%zz&%zzattach=x&%=y", "mailto:a@e.test?subject=100%&body=%zz"},
	{"encoded separators in a value", "mailto:a@e.test?body=x%26attach%3D/etc/passwd", "mailto:a@e.test?body=x%26attach%3D/etc/passwd"},
	{"a ? and nothing after it", "mailto:a@e.test?", "mailto:a@e.test"},
	{"no address, no field", "mailto:", "mailto:"},
	{"no address, only attach", "mailto:?attach=/etc/passwd", "mailto:"},
	{"no address, only a fragment", "mailto:?#&attach=x", "mailto:"},
}

func TestSanitizeMailtoGivesFrostuisResults(t *testing.T) {
	for _, tc := range mailtoCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SanitizeMailto(tc.in)
			if err != nil || got != tc.want {
				t.Fatalf("SanitizeMailto(%q) = %q, %v, want %q", tc.in, got, err, tc.want)
			}
			if again, err := SanitizeMailto(got); err != nil || again != got {
				t.Errorf("SanitizeMailto(%q) = %q, %v: not idempotent", got, again, err)
			}
		})
	}
}

func TestSanitizeMailtoRefusesOtherLinks(t *testing.T) {
	for _, raw := range []string{
		"", "a@e.test", "https://example.org/?attach=x", "file:///etc/passwd", "javascript:alert(1)",
		"mailto:a@e.test?subject=a b", "mailto:a@e.test?subject=x\nattach=y", "mailto:a@e.test\x7f",
		"mailto:a@e.test#%zz", "mailto:a@e.test?\tattach=x",
	} {
		if got, err := SanitizeMailto(raw); !errors.Is(err, errNotAllowed) {
			t.Errorf("SanitizeMailto(%q) = %q, %v, want a refusal", raw, got, err)
		}
	}
}
