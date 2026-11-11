// Package reading is the desktop app's reading view
// (docs/design/app.md#reading-view, plan 0009): notesd renders the note as
// Markdown for native views (note.renderMarkdown,
// docs/specs/render.md#markdown-for-native-views), and MyGo's Markdown
// view draws it, with images read from notesd's blob store by hash
// (ADR-0023) and note embeds and callouts drawn from their fenced blocks.
// MyGo's view shows raw HTML as text, so a note runs nothing.
package reading

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/render"
)

// RenderDelay is how long the view waits after a change of the text
// before it renders again, as the web app's.
const RenderDelay = 300 * time.Millisecond

// maxRemoteImage bounds a remote image's bytes.
const maxRemoteImage = 20 << 20

// maxImagePixels bounds an image's size once decoded, at 4 bytes a pixel.
const maxImagePixels = 64 << 20

// Renderer is what the view needs of notesd; api.NoteClient has it.
type Renderer interface {
	RenderMarkdown(ctx context.Context, p *api.NoteRenderMarkdownParams) (*api.RenderedMarkdown, error)
}

// Links are where the view's links lead; the window wires them.
type Links struct {
	// Note opens a note or file of the vault; newTab with Ctrl held.
	Note func(path, subpath string, newTab bool)
	// Tag searches the vault for a tag.
	Tag func(tag string)
	// URL opens a web link outside the app.
	URL func(url string)
}

// drawer draws notesd's rendered Markdown of a note: what the reading view
// and live preview's blocks share.
type drawer struct {
	doc   *notes.Document
	blobs string // notesd's blob store, read by hash

	Links Links
	// RemoteImages loads images from the web (the Load remote images
	// preference).
	RemoteImages bool
	// Options are the Markdown view's; the view fills in its hooks.
	Options markdown.Options

	folds map[string]bool // callouts the user opened or closed
}

func newDrawer(doc *notes.Document, blobs string) *drawer {
	d := &drawer{doc: doc, blobs: blobs, folds: map[string]bool{}}
	d.Options = markdown.Options{
		Extensions: parser.GFM | parser.ExtFootnotes | parser.ExtMath | parser.ExtDollarMath,
		OnLink:     d.link,
		LoadImage:  d.loadImage,
		Code: map[string]func(*ui.Context, *markdown.CodeBlockView){
			render.EmbedLanguage:   d.embed,
			render.CalloutLanguage: d.callout,
		},
	}
	return d
}

// View is a tab's reading view of a note. UI thread only.
type View struct {
	*drawer
	env notes.Env
	api func() Renderer

	md       string
	rendered string // the text md was rendered from
	err      error
	inFlight bool
	stale    bool // the vault changed since the last rendering asked
	stop     func()
	changed  []func()
}

// New makes the reading view of doc. blobs is notesd's blob store
// (config.Paths.Blobs).
func New(env notes.Env, client func() Renderer, doc *notes.Document, blobs string) *View {
	v := &View{drawer: newDrawer(doc, blobs), env: env, api: client}
	doc.OnChange(v.schedule)
	v.render()
	return v
}

// OnChange calls fn on the UI thread when a new rendering arrives.
func (v *View) OnChange(fn func()) { v.changed = append(v.changed, fn) }

// Markdown is the latest rendering, "" before the first.
func (v *View) Markdown() string { return v.md }

// Err is why the latest rendering failed, nil when it did not.
func (v *View) Err() error { return v.err }

// Changed says the vault changed: what the note embeds may read otherwise.
func (v *View) Changed() {
	v.stale = true
	v.schedule()
}

func (v *View) schedule() {
	if !v.stale && v.doc.Loaded() && v.doc.Text() == v.rendered && v.md != "" {
		return
	}
	if v.stop != nil {
		v.stop()
	}
	v.stop = v.env.After(RenderDelay, func() {
		v.stop = nil
		v.render()
	})
}

// render asks notesd for the note's Markdown: of the text the editor holds
// once loaded, so what was typed shows before it is saved.
func (v *View) render() {
	if v.inFlight {
		return // the answer schedules the next
	}
	c := v.api()
	if c == nil {
		return
	}
	params := &api.NoteRenderMarkdownParams{VaultID: v.doc.Vault(), Path: v.doc.Path()}
	text := v.doc.Text()
	if v.doc.Loaded() {
		params.Content = &text
	}
	v.inFlight, v.stale = true, false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := c.RenderMarkdown(ctx, params)
		v.env.Post(func() {
			v.inFlight = false
			if err != nil {
				v.err = err
			} else {
				v.err, v.md, v.rendered = nil, res.Markdown, text
			}
			for _, fn := range v.changed {
				fn()
			}
			if v.stale || v.doc.Loaded() && v.doc.Text() != v.rendered {
				v.schedule()
			}
		})
	}()
}

// View draws the note.
func (v *View) View(c *ui.Context) ui.Element {
	return markdown.View(c, v.md, &v.Options)
}

func (v *drawer) link(href string) {
	if p, sub, tag, ok := render.NoteLink(href); ok {
		switch {
		case tag != "" && v.Links.Tag != nil:
			v.Links.Tag(tag)
		case tag == "" && v.Links.Note != nil:
			if p == "" {
				p = v.doc.Path() // a heading of the note itself
			}
			v.Links.Note(p, sub, false)
		}
		return
	}
	if v.Links.URL != nil && ui.SafeToOpen(href) {
		v.Links.URL(href)
	}
}

var blobRe = regexp.MustCompile(`^blob:([0-9a-f]{64})\.([a-z0-9]{1,8})$`)

func (v *drawer) loadImage(url string) (ui.ImageSource, error) {
	if m := blobRe.FindStringSubmatch(url); m != nil {
		data, err := os.ReadFile(filepath.Join(v.blobs, m[1][:2], m[1]))
		if err != nil {
			return nil, err
		}
		return Decode(data, m[2])
	}
	if v.RemoteImages && (strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")) {
		return fetch(url)
	}
	return nil, os.ErrNotExist
}

// Decode reads an image of a note or the web. Its size in pixels is checked
// from its header first, as a small file can declare a huge image.
func Decode(data []byte, ext string) (ui.ImageSource, error) {
	if strings.EqualFold(ext, "svg") {
		return ui.ParseSVG(data)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, errors.New("image: larger than " + strconv.Itoa(maxImagePixels>>20) + " megapixels")
	}
	return ui.DecodeBitmap(data)
}

func fetch(url string) (ui.ImageSource, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url) //nolint:noctx // bounded by the client's timeout; LoadImage has no context
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("image: " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteImage+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRemoteImage {
		return nil, errors.New("image: larger than " + strconv.Itoa(maxRemoteImage>>20) + " MB")
	}
	ext := ""
	if strings.Contains(resp.Header.Get("Content-Type"), "svg") {
		ext = "svg"
	}
	return Decode(data, ext)
}

// embed draws a note embed: a quiet box holding the embedded note, with
// its name, which opens it.
func (v *drawer) embed(c *ui.Context, b *markdown.CodeBlockView) {
	t := c.Theme()
	target := strings.TrimSpace(strings.TrimPrefix(b.Info, render.EmbedLanguage))
	p, sub, _ := strings.Cut(target, "#")
	if sub != "" {
		sub = "#" + sub
	}
	name := strings.TrimSuffix(filepath.Base(p), ".md")
	ui.Column(c).Gap(6).Padding(10, 14).Radius(8).Border(1, t.Border).AlignSelf(ui.Stretch).Children(func() {
		open := ui.Text(c, name).FontSize(12).TextColor(t.TextMuted).Role(ui.RoleLink).Label("Open " + name).Cursor(ui.CursorPointer)
		if open.Clicked() && v.Links.Note != nil {
			v.Links.Note(p, sub, false)
		}
		markdown.View(c.Key(b.Index), b.Code(), &v.Options)
	})
}

// calloutColors are the callouts' colors by type, as the web app's
// (app/src/styles/editor.css), for the types Obsidian names.
func calloutTone(t *ui.Theme, typ string) ui.Color {
	switch typ {
	case "warning", "caution", "attention":
		return t.Warning
	case "danger", "error", "bug", "failure", "fail", "missing":
		return t.Danger
	case "success", "check", "done", "tip", "hint", "important":
		return t.Success
	}
	return t.Accent
}

// callout draws a callout: its title in its type's color over its text,
// which a fold mark lets the user open and close.
func (v *drawer) callout(c *ui.Context, b *markdown.CodeBlockView) {
	t := c.Theme()
	info := strings.TrimSpace(strings.TrimPrefix(b.Info, render.CalloutLanguage))
	head, title, _ := strings.Cut(info, " ")
	fold := ""
	if strings.HasSuffix(head, "+") || strings.HasSuffix(head, "-") {
		fold, head = head[len(head)-1:], head[:len(head)-1]
	}
	if title == "" && head != "" {
		title = strings.ToUpper(head[:1]) + head[1:]
	}
	key := strconv.Itoa(b.Index) + info + b.Code() // live preview's blocks share the map
	open, set := v.folds[key]
	if !set {
		open = fold != "-"
	}
	tone := calloutTone(t, head)
	ui.Row(c).Radius(8).Background(tone.Alpha(0.1)).AlignSelf(ui.Stretch).AlignItems(ui.Stretch).Clip().Children(func() {
		ui.Box(c).Width(3).Background(tone)
		ui.Column(c).Grow(1).Gap(6).Padding(10, 14).Children(func() { v.calloutBody(c, b, key, title, fold, tone, open) })
	})
}

func (v *drawer) calloutBody(c *ui.Context, b *markdown.CodeBlockView, key, title, fold string, tone ui.Color, open bool) {
	{
		row := ui.Row(c).Gap(6).AlignItems(ui.Center)
		if fold != "" {
			row.Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(title).Expanded(open)
			if row.Clicked() {
				v.folds[key] = !open
				open = !open
			}
		}
		row.Children(func() {
			ui.Text(c, title).Bold().TextColor(tone)
			if fold != "" {
				mark := "▸"
				if open {
					mark = "▾"
				}
				ui.Text(c, mark).TextColor(tone)
			}
		})
		if open {
			markdown.View(c.Key(b.Index), b.Code(), &v.Options)
		}
	}
}
