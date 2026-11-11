package reading

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// onePixel is a 1×1 PNG.
var onePixel, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

type fixture struct {
	*desktoptest.Notesd
	loop *desktoptest.Loop
	doc  *notes.Document
	view *View
	tt   *ui.Tester
}

func open(t *testing.T, files map[string]string, path string) *fixture {
	t.Helper()
	f := &fixture{Notesd: desktoptest.StartNotesd(t), loop: desktoptest.NewLoop()}
	for p, content := range files {
		f.Create(p, content)
	}
	if _, err := f.Other.Note().Upload(t.Context(), &api.NoteUploadParams{VaultID: f.Vault, Path: "photo.png", Data: base64.StdEncoding.EncodeToString(onePixel)}); err != nil {
		t.Fatal(err)
	}
	client := func() notes.NoteAPI {
		if c := f.AppNotes(); c != nil {
			return c
		}
		return nil
	}
	f.doc = notes.NewDocument(f.loop, client, f.Vault, path)
	f.doc.Load()
	f.loop.Until(t, "the note to load", f.doc.Loaded)
	f.view = New(f.loop, func() Renderer {
		if c := f.AppNotes(); c != nil {
			return c
		}
		return nil
	}, f.doc, f.Blobs)
	f.loop.Until(t, "the first rendering", func() bool { return f.view.Markdown() != "" })
	f.tt = ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { f.view.View(c) })
	}, 800, 900)
	return f
}

func TestTheReadingView(t *testing.T) {
	f := open(t, map[string]string{
		"Plan.md":  "# Plan\n\nSee [[Seeds|the seeds]] and #garden.\n\n> [!warning] Mind the frost\n> Water daily\n\n![[Seeds]]\n\n![[photo.png]]\n",
		"Seeds.md": "Seeds are sown in spring.\n",
	}, "Plan.md")
	for _, want := range []string{"Plan", "the seeds", "#garden", "Mind the frost", "Water daily", "Seeds are sown in spring."} {
		if !f.tt.HasText(want) {
			t.Errorf("the reading view lacks %q: %q", want, f.tt.Texts())
		}
	}
	var opened []string
	var tags []string
	f.view.Links = Links{
		Note: func(p, sub string, _ bool) { opened = append(opened, p+sub) },
		Tag:  func(tag string) { tags = append(tags, tag) },
	}
	if err := f.tt.Click("the seeds"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.Click("#garden"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "Seeds.md" || len(tags) != 1 || tags[0] != "garden" {
		t.Errorf("links opened %q, tags %q", opened, tags)
	}
	if !strings.Contains(f.view.Markdown(), "<blob:") {
		t.Errorf("the image is no blob: %q", f.view.Markdown())
	}
	if img, err := f.view.loadImage(f.view.Markdown()[strings.Index(f.view.Markdown(), "blob:") : strings.Index(f.view.Markdown(), ".png>")+4]); err != nil || img == nil {
		t.Errorf("the image does not load from the blob store: %v", err)
	}
}

func TestTheReadingViewShowsWhatWasTyped(t *testing.T) {
	f := open(t, map[string]string{"Plan.md": "first\n"}, "Plan.md")
	cs, _ := notes.ChangesOf(len(f.doc.Text()), notes.Change{From: 5, To: 5, Insert: " and second"})
	f.doc.Local(cs)
	f.loop.Advance(RenderDelay)
	f.loop.Until(t, "the new rendering", func() bool { return strings.Contains(f.view.Markdown(), "second") })
	f.tt.Frame()
	if !f.tt.HasText("first and second") {
		t.Fatalf("the view shows %q", f.tt.Texts())
	}
}

// hugePNG is a small file whose header declares a huge image.
func hugePNG() []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := append([]byte("IHDR"), binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, 100000), 100000)...)
	chunk = append(chunk, 8, 6, 0, 0, 0) // 8 bits, RGBA
	b.Write(binary.BigEndian.AppendUint32(nil, uint32(len(chunk)-4)))
	b.Write(chunk)
	b.Write(binary.BigEndian.AppendUint32(nil, crc32.ChecksumIEEE(chunk)))
	return b.Bytes()
}

func TestDecodeRefusesAHugeImageFromItsHeader(t *testing.T) {
	if _, err := Decode(hugePNG(), "png"); err == nil || !strings.Contains(err.Error(), "megapixels") {
		t.Fatalf("a huge image: %v", err)
	}
	var small bytes.Buffer
	if err := png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(small.Bytes(), "png"); err != nil {
		t.Fatalf("a small image: %v", err)
	}
}

func TestAnEditOfAnEmbeddedNoteReachesTheReadingView(t *testing.T) {
	f := open(t, map[string]string{"Plan.md": "![[Seeds]]\n", "Seeds.md": "old seeds\n"}, "Plan.md")
	if !strings.Contains(f.view.Markdown(), "old seeds") {
		t.Fatalf("rendered %q", f.view.Markdown())
	}
	f.Write("Seeds.md", f.Read("Seeds.md").Hash, "new seeds\n")
	f.view.Changed()
	f.loop.Advance(RenderDelay)
	f.loop.Until(t, "the new rendering", func() bool { return strings.Contains(f.view.Markdown(), "new seeds") })
}

// pathRenderer answers with the path it was asked for.
type pathRenderer struct{ asked chan string }

func (r pathRenderer) RenderMarkdown(_ context.Context, p *api.NoteRenderMarkdownParams) (*api.RenderedMarkdown, error) {
	r.asked <- p.Path
	return &api.RenderedMarkdown{Markdown: "x"}, nil
}

// Run with the race detector: the request's path is read on the UI thread.
func TestABlockAsksForThePathItHadWhenDrawn(t *testing.T) {
	f := open(t, map[string]string{"Plan.md": "text\n"}, "Plan.md")
	r := pathRenderer{asked: make(chan string, 1)}
	b := NewBlocks(f.loop, func() Renderer { return r }, f.doc, f.Blobs)
	b.rendering("source")
	f.doc.Moved("Moved.md")
	if got := <-r.asked; got != "Plan.md" {
		t.Fatalf("asked for %q", got)
	}
}
