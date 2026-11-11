package reading

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

const (
	// RefreshDelay is how long blocks wait after the vault changes before
	// those on screen render again, so an embedded note follows its edits.
	RefreshDelay = time.Second
	// maxBlocks bounds the renderings kept.
	maxBlocks = 256
)

// Blocks draws pieces of a note as the reading view would, for live
// preview's callouts, tables, embeds and images: notesd renders each as if
// it were the note, and only those on screen are asked for. UI thread only.
type Blocks struct {
	*drawer
	env        notes.Env
	api        func() Renderer
	entries    map[string]*rendering
	generation int
	stop       func()
}

// rendering is what notesd said of one piece of Markdown.
type rendering struct {
	md         string
	done       bool
	inFlight   bool
	generation int
}

// NewBlocks makes the blocks of doc's editor.
func NewBlocks(env notes.Env, client func() Renderer, doc *notes.Document, blobs string) *Blocks {
	return &Blocks{drawer: newDrawer(doc, blobs), env: env, api: client, entries: map[string]*rendering{}, generation: 1}
}

// Changed says the vault changed: what shows renders again a little later.
func (b *Blocks) Changed() {
	if b.stop != nil {
		return
	}
	b.stop = b.env.After(RefreshDelay, func() {
		b.stop = nil
		b.generation++
	})
}

// View draws source, within width when it is not 0; its source, dimmed,
// until notesd has answered.
func (b *Blocks) View(c *ui.Context, source string, width int) {
	b.Options.InlineMath = func(tex string, size float32) (markdown.InlineObject, bool) {
		return b.math.Inline(tex, size, c.Theme().Text)
	}
	r := b.rendering(source)
	if !r.done {
		ui.Text(c, source).TextColor(c.Theme().TextMuted).Wrap()
		return
	}
	col := ui.Column(c).AlignItems(ui.Stretch)
	if width > 0 {
		col.MaxWidth(float32(width))
	}
	col.Children(func() { markdown.View(c, r.md, &b.Options) })
}

func (b *Blocks) rendering(source string) *rendering {
	r := b.entries[source]
	if r == nil {
		if len(b.entries) >= maxBlocks {
			clear(b.entries)
		}
		r = &rendering{}
		b.entries[source] = r
	}
	if r.generation != b.generation && !r.inFlight {
		b.render(source, r)
	}
	return r
}

func (b *Blocks) render(source string, r *rendering) {
	client := b.api()
	if client == nil {
		return
	}
	r.inFlight = true
	generation := b.generation
	params := &api.NoteRenderMarkdownParams{VaultID: b.doc.Vault(), Path: b.doc.Path(), Content: &source}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := client.RenderMarkdown(ctx, params)
		b.env.Post(func() {
			r.inFlight, r.generation = false, generation
			if err == nil {
				r.md, r.done = res.Markdown, true
			}
		})
	}()
}
