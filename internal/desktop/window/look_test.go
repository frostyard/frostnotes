package window

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// fontSample is text whose width tells Inter from the default sans.
const fontSample = "Frostnotes keeps every note in plain Markdown 0123456789"

// lookNote is a note with what the look changes: headings, inline code, a
// fence and a callout.
const lookNote = "# Garden\n\nSome **bold** text, `inline code` and a [[Link]].\n\n## Beds\n\n" +
	"```go\nfunc water(bed int) {\n\tprintln(\"water\", bed)\n}\n```\n\n" +
	"> [!tip] Rotate the beds\n> Beans after squash.\n\n### Tasks\n\n- [ ] sow\n- [x] dig\n"

// TestTheWindowDrawsInTheFrostyardThemes draws the main window over a vault
// in frostui's two themes (its ADR-0011), as the desktop's light or dark
// picks them and as the Theme setting overrides it, and checks what it
// draws with: the theme's ground and chrome, Inter, and in live preview
// headings in Inter and code on the code color. FROSTNOTES_SCREENS keeps
// its pictures for a review by eye.
func TestTheWindowDrawsInTheFrostyardThemes(t *testing.T) {
	dir := os.Getenv("FROSTNOTES_SCREENS")
	frostui.SetIcons(frostui.NewIcons(nil))
	for _, tc := range []struct {
		name, setting string
		desktopDark   bool
		dark          bool
	}{
		{"light", "system", false, false},
		{"dark", "system", true, true},
		{"set-dark", "dark", false, true},
		{"set-light", "light", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := start(t)
			f.tt.SetDark(tc.desktopDark)
			f.a.Prefs.Theme = tc.setting
			f.Create("Garden plan.md", lookNote)
			f.Create("Link.md", "x\n")
			f.openVault()
			f.until("the catalog", func() bool { return f.a.hasFile("Garden plan.md") })
			f.a.open("Garden plan.md", workspace.Here)
			f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
			e := f.a.activeEditor()
			end := len(e.Binding().Document().Text())
			e.State.Select(end, end)
			for range 5 {
				f.loop.Advance(time.Second)
				f.tt.Frame()
				f.loop.Drain()
			}

			want := frostui.FrostyardTheme(tc.dark)
			th := f.theme
			if th.Dark != tc.dark || th.Background != want.Background {
				t.Fatalf("the window drew on %v (dark %v), want Frostyard's %v (dark %v)", th.Background, th.Dark, want.Background, tc.dark)
			}
			if th.Font != frostui.FontFamily {
				t.Errorf("the window's font is %q, want %q", th.Font, frostui.FontFamily)
			}
			if !slices.ContainsFunc(ui.FontFamilies(), func(ff ui.FontFamily) bool { return ff.Name == frostui.FontFamily }) {
				t.Errorf("no %q among the text system's families", frostui.FontFamily)
			}
			if f.inter == f.sans {
				t.Errorf("text in %q is as wide as the default sans (%v): the window draws in a fallback", frostui.FontFamily, f.inter)
			}
			k := frostui.ColorsOf(th)
			img := f.tt.Image()
			// The note's pane, below its text, is the ground; the left
			// sidebar under the files is the chrome.
			if got := commonest(img, image.Rect(300, 640, 880, 760)); !near(got, th.Background) {
				t.Errorf("the note's pane is %v, want the background %v", got, th.Background)
			}
			if got := commonest(img, image.Rect(10, 400, 250, 760)); !near(got, k.Chrome) {
				t.Errorf("the left sidebar is %v, want the chrome %v", got, k.Chrome)
			}

			// Live preview: the heading "## Beds" in Inter, the fence on the
			// code color.
			var heading bool
			for _, s := range e.State.LineStyles() {
				if s.Line == 4 {
					heading = s.Font.Family == frostui.FontFamily && s.Font.Weight == 700
				}
			}
			if !heading {
				t.Errorf("line 4, a heading, is not in bold %s: %+v", frostui.FontFamily, e.State.LineStyles())
			}
			if bg := e.State.LineBackgrounds; len(bg) != 1 || bg[0].First != 6 || bg[0].Last != 10 || bg[0].Color != k.Code {
				t.Errorf("the fence's lines are on %+v, want lines 6 to 10 on the code color %v", bg, k.Code)
			}
			save(t, dir, "look-live-"+tc.name, img)

			// The zoom stays a zoom of the whole window, in the same theme.
			f.a.run("window:zoom-in")
			f.tt.Frame()
			if f.zoom != 1.1 || f.theme.Background != want.Background || f.theme.FontSize != want.FontSize {
				t.Errorf("zoomed in: zoom %v, ground %v, text size %v; want 1.1 on %v at %v", f.zoom, f.theme.Background, f.theme.FontSize, want.Background, want.FontSize)
			}
			f.a.run("window:reset-zoom")
			f.tt.Frame()

			f.a.run("markdown:toggle-preview")
			f.until("the reading view", func() bool { return f.a.activeEditor() == nil })
			f.loop.Advance(time.Second)
			f.until("the rendering", func() bool { return f.a.activeContent().read.Markdown() != "" })
			f.tt.Frame()
			if look := f.a.activeContent().read.Options.Theme; look == nil || look.CodeBlockBackground != k.Code {
				t.Errorf("the reading view's code blocks are on %+v, want the code color %v", look, k.Code)
			}
			save(t, dir, "look-reading-"+tc.name, f.tt.Image())
			f.a.Session.Close()
		})
	}
}

// commonest is the color most pixels of r in img have.
func commonest(img *image.RGBA, r image.Rectangle) color.RGBA {
	counts := map[color.RGBA]int{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			counts[img.RGBAAt(x, y)]++
		}
	}
	var best color.RGBA
	for c, n := range counts {
		if n > counts[best] {
			best = c
		}
	}
	return best
}

// near reports whether a drawn pixel is c, within rounding.
func near(got color.RGBA, c ui.Color) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) <= 2 && int(y)-int(x) <= 2 }
	return got.A == 255 && d(got.R, c.R) && d(got.G, c.G) && d(got.B, c.B)
}

// save writes img to dir as name.png, when dir is set.
func save(t *testing.T, dir, name string, img image.Image) {
	t.Helper()
	if dir == "" {
		return
	}
	out, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	if err := png.Encode(out, img); err != nil {
		t.Fatal(err)
	}
}
