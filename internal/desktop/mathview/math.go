// Package mathview lays out TeX as bounded, cached vector images for native views.
package mathview

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/d2lang/mathjax-go"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const maxEntries = 128
const maxSource = 2048

type key struct {
	source  string
	size    float32
	display bool
}
type formula struct {
	svg           *ui.SVG
	width, height float32
}

// Cache belongs to one view and the UI thread. Failed formulas keep their source.
type Cache struct{ entries map[key]*formula }

func (c *Cache) get(tex string, size float32) *formula { return c.formula(tex, size, false) }

func (c *Cache) formula(tex string, size float32, display bool) *formula {
	k := key{tex, size, display}
	if f, ok := c.entries[k]; ok {
		return f
	}
	if c.entries == nil || len(c.entries) >= maxEntries {
		c.entries = map[key]*formula{}
	}
	f, _ := layoutMode(tex, size, display)
	c.entries[k] = f
	return f
}

// Inline supplies MyGo's math hook. Color is the surrounding theme's text color.
func (c *Cache) Inline(tex string, size float32, color ui.Color) (markdown.InlineObject, bool) {
	f := c.get(tex, size)
	if f == nil {
		return markdown.InlineObject{}, false
	}
	return markdown.InlineObject{Width: f.width, Height: f.height, Draw: func(p *ui.Painter, r ui.Rect) { p.Icon(f.svg, r, color) }}, true
}

// View draws display math, keeping invalid or unsupported expressions as source.
func (c *Cache) View(ctx *ui.Context, tex string) {
	size := ctx.Theme().FontSize
	f := c.formula(tex, size, true)
	if f == nil {
		ui.Text(ctx, tex).Font("monospace").Wrap()
		return
	}
	color := ctx.Theme().Text
	ui.Box(ctx).Width(f.width).Height(f.height).Label(tex).Draw(func(p *ui.Painter, r ui.Rect) { p.Icon(f.svg, r, color) })
}

func layout(tex string, size float32) (*formula, error) { return layoutMode(tex, size, false) }

func layoutMode(tex string, size float32, display bool) (f *formula, err error) {
	defer func() {
		if cause := recover(); cause != nil {
			f = nil
			err = fmt.Errorf("math: invalid expression: %v", cause)
		}
	}()
	if len(tex) > maxSource || size <= 0 || size > 128 || math.IsNaN(float64(size)) {
		return nil, errors.New("math: expression exceeds limits")
	}
	depth := 0
	for _, r := range tex {
		if r == '{' {
			depth++
			if depth > 32 {
				return nil, errors.New("math: nesting exceeds limit")
			}
		}
		if r == '}' {
			depth--
		}
	}
	output, err := mathjax.RenderWithOptions(tex, mathjax.Options{Em: float64(size), Ex: float64(size) / 2, Display: display})
	if err != nil {
		return nil, err
	}
	if strings.Contains(output, `data-mml-node="merror"`) || len(output) > 2<<20 {
		return nil, errors.New("math: unsupported expression")
	}
	dimensions := svgDimensions.FindStringSubmatch(output)
	if len(dimensions) != 3 {
		return nil, errors.New("math: missing dimensions")
	}
	w, err := strconv.ParseFloat(dimensions[1], 64)
	if err != nil {
		return nil, err
	}
	h, err := strconv.ParseFloat(dimensions[2], 64)
	if err != nil {
		return nil, err
	}
	w, h = w*float64(size)/2, h*float64(size)/2
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, errors.New("math: layout exceeds limits")
	}
	output = strings.Replace(output, `width="`+dimensions[1]+`ex"`, fmt.Sprintf(`width="%g"`, w), 1)
	output = strings.Replace(output, `height="`+dimensions[2]+`ex"`, fmt.Sprintf(`height="%g"`, h), 1)
	svg, err := ui.ParseSVG([]byte(output))
	if err != nil {
		return nil, err
	}
	return &formula{svg: svg, width: float32(w), height: float32(h)}, nil
}

var svgDimensions = regexp.MustCompile(`width="([0-9.]+)ex" height="([0-9.]+)ex"`)
