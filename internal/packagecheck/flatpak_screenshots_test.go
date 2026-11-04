package packagecheck

// Software centres show the default screenshot first; appstreamcli validate
// fails on a missing default but only gives an info for an http URL; so this
// test checks both, and that each URL names a PNG in this repository on main,
// so a renamed or deleted image fails here before a software centre shows a
// broken link.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pngSignature = "\x89PNG\r\n\x1a\n"

type flatpakScreenshotMeta struct {
	Screenshots struct {
		Items []struct {
			Type    string   `xml:"type,attr"`
			Caption []string `xml:"caption"`
			Images  []string `xml:"image"`
		} `xml:"screenshot"`
	} `xml:"screenshots"`
}

func checkScreenshots(meta []byte, root string) []error {
	var doc flatpakScreenshotMeta
	if err := xml.Unmarshal(meta, &doc); err != nil {
		return []error{fmt.Errorf("metainfo XML: %w", err)}
	}

	var errs []error
	screenshots := doc.Screenshots.Items
	if len(screenshots) == 0 {
		errs = append(errs, fmt.Errorf("metainfo has no screenshot"))
		return errs
	}

	if screenshots[0].Type != "default" {
		errs = append(errs, fmt.Errorf("the first screenshot is missing type=\"default\""))
	}
	for i, screenshot := range screenshots[1:] {
		if screenshot.Type == "default" {
			errs = append(errs, fmt.Errorf("screenshot %d also has type=\"default\"", i+2))
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		errs = append(errs, fmt.Errorf("resolve repository root: %w", err))
		return errs
	}

	const mainPrefix = "/frostyard/frostnotes/main/"
	for i, screenshot := range screenshots {
		if len(screenshot.Caption) == 0 || strings.TrimSpace(screenshot.Caption[0]) == "" {
			errs = append(errs, fmt.Errorf("screenshot %d has an empty caption", i+1))
		}
		if len(screenshot.Images) == 0 {
			errs = append(errs, fmt.Errorf("screenshot %d has no image", i+1))
		}

		for j, rawURL := range screenshot.Images {
			u, err := url.Parse(strings.TrimSpace(rawURL))
			if err != nil {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL is invalid: %w", i+1, j+1, err))
				continue
			}
			if u.Scheme != "https" {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL scheme is %q, want https", i+1, j+1, u.Scheme))
			}
			if u.Host != "raw.githubusercontent.com" {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL host is %q, want raw.githubusercontent.com", i+1, j+1, u.Host))
			}
			if !strings.HasPrefix(u.Path, mainPrefix) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL path %q does not start with %q", i+1, j+1, u.Path, mainPrefix))
				continue
			}

			rest := strings.TrimPrefix(u.Path, mainPrefix)
			target := filepath.Join(absRoot, filepath.FromSlash(rest))
			rel, err := filepath.Rel(absRoot, target)
			if err != nil {
				errs = append(errs, fmt.Errorf("screenshot %d image %d path is invalid: %w", i+1, j+1, err))
				continue
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d path is outside the repository root", i+1, j+1))
				continue
			}

			data, err := os.ReadFile(target)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					errs = append(errs, fmt.Errorf("screenshot %d image %d file does not exist: %s", i+1, j+1, filepath.ToSlash(rel)))
				} else {
					errs = append(errs, fmt.Errorf("screenshot %d image %d file cannot be read: %w", i+1, j+1, err))
				}
				continue
			}
			if !strings.HasPrefix(string(data), pngSignature) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d file is not a PNG: %s", i+1, j+1, filepath.ToSlash(rel)))
			}
		}
	}

	return errs
}

func TestFlatpakMetainfoScreenshots(t *testing.T) {
	meta, err := os.ReadFile(filepath.Join("..", "..", "packaging", "flatpak", flatpakMetainfo))
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkScreenshots(meta, "../.."); len(errs) != 0 {
		for _, err := range errs {
			t.Error(err)
		}
	}
}

func TestFlatpakMetainfoScreenshotsRejects(t *testing.T) {
	root := t.TempDir()
	base := "https://raw.githubusercontent.com/frostyard/frostnotes/main/docs/images/"

	validPath := filepath.Join(root, "docs", "images", "editor.png")
	if err := os.MkdirAll(filepath.Dir(validPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(validPath, []byte(pngSignature+"valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	notPNGPath := filepath.Join(root, "docs", "images", "not-png.png")
	if err := os.WriteFile(notPNGPath, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}

	component := func(body string) string {
		return "<component>" + body + "</component>"
	}
	screenshot := func(typ, caption, image string) string {
		var attrs string
		if typ != "" {
			attrs = ` type="` + typ + `"`
		}
		var parts strings.Builder
		parts.WriteString("<screenshot" + attrs + ">")
		if caption != "" {
			parts.WriteString("<caption>" + caption + "</caption>")
		} else {
			parts.WriteString("<caption></caption>")
		}
		if image != "" {
			parts.WriteString("<image>" + image + "</image>")
		}
		parts.WriteString("</screenshot>")
		return parts.String()
	}

	valid := base + "editor.png"
	cases := []struct {
		name string
		meta string
		want string
	}{
		{"no screenshots", component(""), "no screenshot"},
		{"first is not default", component("<screenshots>" + screenshot("", "Editor", valid) + "</screenshots>"), "default"},
		{"two defaults", component("<screenshots>" + screenshot("default", "One", valid) + screenshot("default", "Two", valid) + "</screenshots>"), "default"},
		{"empty caption", component("<screenshots>" + screenshot("default", "", valid) + "</screenshots>"), "caption"},
		{"no image", component("<screenshots>" + screenshot("default", "Editor", "") + "</screenshots>"), "image"},
		{"http URL", component("<screenshots>" + screenshot("default", "Editor", "http://raw.githubusercontent.com/frostyard/frostnotes/main/docs/images/editor.png") + "</screenshots>"), "scheme"},
		{"another host", component("<screenshots>" + screenshot("default", "Editor", "https://example.com/frostyard/frostnotes/main/docs/images/editor.png") + "</screenshots>"), "host"},
		{"outside main", component("<screenshots>" + screenshot("default", "Editor", "https://raw.githubusercontent.com/other/repository/main/docs/images/editor.png") + "</screenshots>"), "main/"},
		{"file does not exist", component("<screenshots>" + screenshot("default", "Editor", base+"missing.png") + "</screenshots>"), "does not exist"},
		{"file is not PNG", component("<screenshots>" + screenshot("default", "Editor", base+"not-png.png") + "</screenshots>"), "PNG"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := checkScreenshots([]byte(tc.meta), root)
			if len(errs) == 0 {
				t.Fatalf("checkScreenshots accepted bad metainfo; want error naming %q", tc.want)
			}
			for _, err := range errs {
				if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
					return
				}
			}
			t.Errorf("errors do not name %q: %v", tc.want, errs)
		})
	}
}
