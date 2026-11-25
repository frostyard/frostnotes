package system

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Opening things outside the app (docs/design/app.md#window): a web link in
// the system's browser (http, https and mailto only), and a vault file in
// the system's default app. A file must be a regular, non-executable file
// inside the vault's folder, of a type that opens as a document, never one
// a desktop runs (a script, a .desktop entry, HTML). These are the app's
// rules: the window checks every link with WebURL, and cmd/frostnotes then
// opens what passes through frostui's Opener, which checks it again. This
// file imports no UI toolkit, so a client on another
// toolkit keeps the same rules.
// The extension list guards a file's type, not the handler: the system may
// pick one by the file's content. A vault on a CIFS mount whose file_mode has execute bits
// (the kernel's default, 0755) shows every file executable, so Open in
// default app refuses them all; mount it with file_mode=0644.

// OpenableExtensions are the extensions Open in default app opens:
// documents and media, never anything a desktop runs or a browser scripts.
func OpenableExtensions() []string {
	return []string{
		".md", ".txt", ".pdf", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".avif", ".mp3", ".wav", ".ogg", ".flac", ".m4a",
		".webm", ".mp4", ".mov", ".mkv", ".ogv", ".3gp", ".canvas", ".base", ".csv", ".odt", ".ods", ".odp", ".doc", ".docx", ".xls",
		".xlsx", ".ppt", ".pptx", ".rtf", ".epub",
	}
}

// WebURL checks that raw may open in the system's browser and returns what
// to open: an http or https link with a host, or a mailto link as
// SanitizeMailto returns it, by frostui.SanitizeMailto's rule (frostui's
// ADR-0010). That keeps only its addressees, subject and body, and drops
// whatever else it asks of the mail client, such as attaching a file, and
// its fragment, which a client might read as more fields (#200).
func WebURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("not a URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Hostname() == "" {
			return "", fmt.Errorf("%s: a web link needs a host", raw)
		}
		return u.String(), nil
	case "mailto":
		return SanitizeMailto(raw)
	}
	return "", fmt.Errorf("%s: links open outside the app only for http, https and mailto", u.Scheme)
}

// errNotAllowed is what SanitizeMailto's refusals wrap.
var errNotAllowed = errors.New("not allowed")

// refused is an error saying why target is not opened, in frostui's
// words, so the app says what it said when frostui made the check.
func refused(target, why string) error {
	return fmt.Errorf("frostui: refused to open %q: %s: %w", target, why, errNotAllowed)
}

// mailtoFields are the header fields a mailto link keeps.
var mailtoFields = []string{"to", "cc", "bcc", "subject", "body"}

// isMailtoField reports whether name is one of mailtoFields, in any ASCII
// case and not encoded. ASCII alone: Unicode folding would read "ſubject"
// as "subject".
func isMailtoField(name string) bool {
	return slices.ContainsFunc(mailtoFields, func(field string) bool {
		if len(name) != len(field) {
			return false
		}
		for i := range len(field) {
			// field[i] is a lowercase letter: only it and its capital
			// give it with the 0x20 bit set.
			if name[i]|0x20 != field[i] {
				return false
			}
		}
		return true
	})
}

// SanitizeMailto is frostui.SanitizeMailto (frostui v0.10.1), carried here
// so that system imports no UI toolkit; a test of cmd/frostnotes holds the
// two to the same results. It returns the mailto link raw with only its
// addressees, subject and body: the address and every field named to, cc,
// bcc, subject or body, in any ASCII case, as written and in order. It
// removes, rather than refuses, every other field (attach=), a field whose
// name is encoded (%61ttach) or has no "=", the fragment, and a "?" with
// nothing after it. It decodes nothing: a kept value passes byte for byte,
// "+" and malformed escapes included. It refuses what is not a mailto link
// or holds spaces or control characters.
func SanitizeMailto(raw string) (string, error) {
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return "", refused(raw, "it holds spaces or control characters")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", refused(raw, "it is not a URL")
	}
	if !strings.EqualFold(u.Scheme, "mailto") {
		return "", refused(raw, "it is not a mailto link")
	}
	var kept []string
	for field := range strings.SplitSeq(u.RawQuery, "&") {
		if name, _, ok := strings.Cut(field, "="); ok && isMailtoField(name) {
			kept = append(kept, field)
		}
	}
	u.RawQuery = strings.Join(kept, "&")
	u.ForceQuery = false
	u.Fragment, u.RawFragment = "", ""
	return u.String(), nil
}

// VaultFile resolves p inside the vault folder root to a file Open in
// default app may open.
func VaultFile(root, p string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", errors.New("the vault's folder is not an absolute path")
	}
	if p == "" || !filepath.IsLocal(p) || path.Clean(p) != p {
		return "", fmt.Errorf("%s: not a path inside the vault", p)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("the vault's folder: %w", err)
	}
	file, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(p)))
	if err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	if !strings.HasPrefix(file, real+string(filepath.Separator)) {
		return "", fmt.Errorf("%s leads outside the vault", p)
	}
	info, err := os.Stat(file)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", p)
	}
	if info.Mode().Perm()&fs.FileMode(0o111) != 0 {
		return "", fmt.Errorf("%s is executable: the app does not run files", p)
	}
	if !slices.Contains(OpenableExtensions(), strings.ToLower(filepath.Ext(file))) {
		return "", fmt.Errorf("%s: the app opens only documents and media outside itself", p)
	}
	return file, nil
}
