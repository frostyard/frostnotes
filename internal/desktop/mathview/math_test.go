package mathview

import (
	"image/color"
	"strings"
	"testing"
)

func TestFormulaDrawsVectorsAndRejectsInvalidSource(t *testing.T) {
	for _, src := range []string{`x_i^2`, `\frac{a+b}{2}`, `\sqrt{x}`, `\sum_{i=1}^{n} i`} {
		f, err := layout(src, 16)
		if err != nil || f == nil {
			t.Fatalf("%s: %v", src, err)
		}
		img := f.svg.Image(int(f.width+1), int(f.height+1), color.Black)
		ink := 0
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				_, _, _, a := img.At(x, y).RGBA()
				if a > 0 {
					ink++
				}
			}
		}
		if ink == 0 {
			t.Fatalf("%s rendered no ink", src)
		}
	}
	for _, src := range []string{strings.Repeat("x", maxSource+1), strings.Repeat("{", 33), `\unsupportedcommand{x}`} {
		if f, _ := layout(src, 16); f != nil {
			t.Fatalf("accepted %q", src)
		}
	}
}

func TestCacheReusesAndBoundsFormulas(t *testing.T) {
	var c Cache
	f := c.get("x", 16)
	if f == nil || c.get("x", 16) != f {
		t.Fatal("not cached")
	}
	for i := range maxEntries + 5 {
		c.get(strings.Repeat("x", i+1), 16)
	}
	if len(c.entries) > maxEntries {
		t.Fatal("unbounded cache")
	}
}
