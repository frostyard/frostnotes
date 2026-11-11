package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSourceOnlyOpensMediaBlobs(t *testing.T) {
	root := t.TempDir()
	hash := strings.Repeat("a", 64)
	path, kind, err := Source(root, "blob:"+hash+".mp4")
	if err != nil || kind != "video" || path != filepath.Join(root, "aa", hash) {
		t.Fatalf("%q %q %v", path, kind, err)
	}
	for _, source := range []string{"https://example.com/a.mp4", "file:///tmp/a.mp4", "blob:../../a.mp4", "blob:" + hash + ".m3u", "blob:" + hash + ".png", "blob:" + hash + ".mp4/../x"} {
		if _, _, err := Source(root, source); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}

type fakeTransport struct {
	closed int
	active bool
	at     time.Duration
}

func (p *fakeTransport) Play()                   { p.active = true }
func (p *fakeTransport) Pause()                  { p.active = false }
func (p *fakeTransport) Playing() bool           { return p.active }
func (p *fakeTransport) Seek(d time.Duration)    { p.at = d }
func (p *fakeTransport) Position() time.Duration { return p.at }
func (p *fakeTransport) Duration() time.Duration { return time.Minute }
func (p *fakeTransport) Close()                  { p.closed++ }

func TestUnmountClosesPlaybackAndFileOnce(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "media")
	if err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{}
	show := true
	tt := ui.NewTester(func(c *ui.Context) {
		if show {
			box := ui.Column(c).Key("media")
			ui.Local(box, "player", func() player { return player{transport: transport, file: file} })
		}
	}, 400, 200)
	show = false
	tt.Frame()
	tt.Frame()
	if transport.closed != 1 {
		t.Fatalf("closed %d times", transport.closed)
	}
	if _, err := file.Stat(); err == nil {
		t.Fatal("file remains open")
	}
}

func TestViewWaitsForPlayBeforeOpening(t *testing.T) {
	url := "blob:" + strings.Repeat("a", 64) + ".wav"
	root := t.TempDir()
	tt := ui.NewTester(func(c *ui.Context) { View(c, root, url, "Clip") }, 400, 200)
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	// The missing-file error is handled by the controls; the view remains usable.
	tt.Frame()
}
