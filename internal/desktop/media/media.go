// Package media plays vault attachments by blob hash, only after a user press.
package media

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/aac"
	_ "github.com/egoist/mygo/audio/flac"
	_ "github.com/egoist/mygo/audio/mp3"
	_ "github.com/egoist/mygo/audio/vorbis"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

// Kind returns audio or video for supported attachment extensions.
func Kind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".wav", ".m4a", ".3gp", ".flac", ".ogg", ".oga", ".opus":
		return "audio"
	case ".mp4", ".webm", ".ogv", ".mov", ".mkv":
		return "video"
	}
	return ""
}

var blobPattern = regexp.MustCompile(`^blob:([0-9a-f]{64})\.([a-z0-9]{1,8})$`)

// Source accepts only a media blob identifier, never a path or a remote URL.
func Source(blobs, url string) (string, string, error) {
	m := blobPattern.FindStringSubmatch(url)
	if m == nil || Kind("x."+m[2]) == "" {
		return "", "", errors.New("media: invalid attachment")
	}
	return filepath.Join(blobs, m[1][:2], m[1]), Kind("x." + m[2]), nil
}

type transport interface {
	Play()
	Pause()
	Playing() bool
	Seek(time.Duration)
	Position() time.Duration
	Duration() time.Duration
	Close()
}

type player struct {
	transport transport
	video     *ui.VideoPlayer
	file      *os.File
	err       error
}

func (p *player) Close() error {
	if p.transport != nil {
		p.transport.Close()
		p.transport = nil
	}
	p.video = nil
	if p.file != nil {
		err := p.file.Close()
		p.file = nil
		return err
	}
	return nil
}
func (p *player) open(path, kind string) {
	if kind == "video" {
		p.video, p.err = video.Open(path)
		if p.err == nil {
			p.transport = p.video
		}
	} else {
		p.file, p.err = os.Open(path)
		if p.err == nil {
			p.transport, p.err = audio.OpenStream(p.file)
		}
	}
	if p.err != nil {
		_ = p.Close()
		return
	}
	p.transport.Play()
}

// View mounts playback controls. Leaving the view closes the player and file.
// Neither files nor system decoders open until Play is pressed.
func View(c *ui.Context, blobs, url, label string) {
	path, kind, err := Source(blobs, url)
	if err != nil {
		ui.Text(c, "Media unavailable").TextColor(c.Theme().TextMuted)
		return
	}
	box := ui.Column(c).Key(url).Gap(8).AlignItems(ui.Stretch).MaxWidth(720)
	p := ui.Local(box, "player", func() player { return player{} })
	box.Children(func() {
		if label != "" {
			ui.Text(c, label).TextColor(c.Theme().TextMuted)
		}
		if p.video != nil {
			ui.Video(c, p.video).FillWidth().Height(320).Fit(ui.Contain).Label(label)
			if p.video.Err() != nil {
				ui.Text(c, p.video.Err().Error()).TextColor(c.Theme().Danger)
			}
		}
		if p.err != nil {
			ui.Text(c, p.err.Error()).TextColor(c.Theme().Danger)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			playing := p.transport != nil && p.transport.Playing()
			name := "Play"
			if playing {
				name = "Pause"
			}
			if ui.Button(c, name).Clicked() {
				switch {
				case p.transport == nil:
					p.open(path, kind)
				case playing:
					p.transport.Pause()
				default:
					p.transport.Play()
				}
			}
			if p.transport != nil {
				pos, dur := p.transport.Position().Seconds(), p.transport.Duration().Seconds()
				if ui.Slider(c, &pos, 0, max(dur, 0.001)).Grow(1).Label("Position").Changed() {
					p.transport.Seek(time.Duration(pos * float64(time.Second)))
				}
				ui.Text(c, clock(p.transport.Position())+" / "+clock(p.transport.Duration()))
				if ui.Button(c, "Stop").Clicked() {
					_ = p.Close()
				}
			}
		})
	})
}
func clock(d time.Duration) string {
	s := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
