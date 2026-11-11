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
// rules; cmd/frostnotes then opens what passes through frostui's Opener.
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
// to open: a mailto link keeps only its addressees, subject and body, and
// loses whatever else it asks of the mail client, such as attaching a file.
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
		var kept []string
		for part := range strings.SplitSeq(u.RawQuery, "&") {
			key, _, _ := strings.Cut(part, "=")
			if slices.Contains([]string{"to", "cc", "bcc", "subject", "body"}, strings.ToLower(key)) {
				kept = append(kept, part)
			}
		}
		u.RawQuery = strings.Join(kept, "&")
		return u.String(), nil
	}
	return "", fmt.Errorf("%s: links open outside the app only for http, https and mailto", u.Scheme)
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
