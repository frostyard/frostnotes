// Command frostnotes is the native desktop app (plan 0009): a window on
// MyGo and frostui over notesd's socket. The window itself is
// internal/desktop/window.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostnotes/internal/desktop/system"
	"github.com/frostyard/frostnotes/internal/desktop/window"
)

//go:embed logo.svg
var logo []byte

// version and commit are stamped at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "frostnotes:", err)
		os.Exit(1)
	}
}

// wakeEvery is how often a burst of posts runs: MyGo rebuilds the view on
// every Window.Update, and a scan's events come by the thousand.
const wakeEvery = 8 * time.Millisecond

// poster hands functions to the window's UI thread. It is safe from any
// goroutine, runs them in order, and keeps what is posted before the window
// exists until attach, so an early result is never lost.
type poster struct {
	mu      sync.Mutex
	waker   *frostui.Waker
	pending []func()
}

// Post runs f on the UI thread.
func (p *poster) Post(f func()) {
	p.mu.Lock()
	p.pending = append(p.pending, f)
	w := p.waker
	p.mu.Unlock()
	if w != nil {
		w.Wake()
	}
}

func (p *poster) drain() {
	p.mu.Lock()
	pending := p.pending
	p.pending = nil
	p.mu.Unlock()
	for _, f := range pending {
		f()
	}
}

// After runs f on the UI thread after d; the function it returns stops it.
func (p *poster) After(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, func() { p.Post(f) })
	return func() { t.Stop() }
}

// Now is the time.
func (*poster) Now() time.Time { return time.Now() }

func (p *poster) attach(w *mygo.Window) {
	waker := frostui.NewWaker(func(wake func()) {
		w.Update(func() {
			p.drain()
			wake()
		})
	}, wakeEvery)
	p.mu.Lock()
	p.waker = waker
	p.mu.Unlock()
	waker.Wake()
}

// clipboard reads files and images from the system's clipboard.
type clipboard struct{}

func (clipboard) Files() []string {
	d, err := mygo.Clipboard.Read(transfer.FileList, transfer.URIList)
	if err != nil {
		return nil
	}
	files, err := d.Files()
	if err != nil {
		return nil
	}
	return files
}

func (clipboard) PNG() []byte { return mygo.Clipboard.ReadImage() }

func pickFolder(title string) string {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: title, Directory: true, CreateDirectories: true})
	if err != nil || len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func pickFiles() []string {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Attach file", Multiple: true})
	if err != nil {
		return nil
	}
	return paths
}

// openURL opens a web link in the system's browser, behind the app's
// rules and frostui's allow-list.
func openURL(raw string) error {
	target, err := system.WebURL(raw)
	if err != nil {
		return err
	}
	return frostui.Opener{}.OpenURL(target)
}

// openFile opens a vault file in the system's default app, behind the
// app's rules and frostui's allow-list.
func openFile(root, p string) error {
	file, err := system.VaultFile(root, p)
	if err != nil {
		return err
	}
	return frostui.Opener{Roots: []string{root}, Extensions: system.OpenableExtensions()}.OpenFile(file)
}

func run() error {
	getenv := os.Getenv
	paths, err := config.Resolve(getenv)
	if err != nil {
		return err
	}
	configDir, err := system.ConfigDir(getenv)
	if err != nil {
		return err
	}
	stateDir, err := system.StateDir(getenv)
	if err != nil {
		return err
	}
	// Problem records go to the local journal, and nowhere else (ADR-0036).
	rec := problems.New(config.AppID, strings.TrimPrefix(version, "v"), problems.ComponentGUI)
	if err := rec.Err(); err != nil {
		slog.Debug("problem records are off", "err", err)
	} else if err := rec.CrashOutput(); err != nil {
		slog.Warn("a crash will not be recorded", "err", err)
	}
	defer rec.RecoverPanic()

	if err := frostui.SetAppID(config.AppID, "Frostnotes"); err != nil {
		slog.Warn("cannot set the app ID", "err", err)
	}
	mygo.App.SetName(config.AppID)
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running app was asked to show its window
	}

	frostui.SetIcons(frostui.NewIcons(map[string][]byte{config.AppID + "-symbolic": logo}))

	var win *mygo.Window
	mygo.App.OnSecondInstance(func([]string, string) {
		if win != nil {
			win.Show()
			win.Focus()
		}
	})
	mygo.App.WhenReady(func() { win = open(getenv, paths.Socket, paths.Blobs, configDir, stateDir) })
	err = mygo.App.Run()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	system.StopOnExit(ctx, getenv)
	return err
}

func open(getenv system.Getenv, socket, blobs, configDir, stateDir string) *mygo.Window {
	p := &poster{}
	var win *mygo.Window
	app := window.New(window.Config{
		Env: p, Dial: session.SocketDialer(socket), Getenv: getenv, Blobs: blobs, Version: version + " (" + commit + ")",
		ConfigAt: configDir, StateAt: stateDir,
		SetTitle: func(t string) {
			if win != nil {
				win.SetTitle(t)
			}
		},
		Close: func() {
			if win != nil {
				win.Close()
			}
		},
		Quit:       mygo.App.Quit,
		PickFolder: pickFolder,
		PickFiles:  pickFiles,
		OpenURL:    openURL,
		OpenFile:   openFile,
		Copy:       mygo.Clipboard.WriteText,
		Clipboard:  clipboard{},
	})
	view := func(c *ui.Context) { app.View(c) }
	win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Frostnotes", Width: 1200, Height: 780, MinWidth: 640, MinHeight: 400,
		TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(view),
	})
	closing := false
	win.OnClose(func(e *mygo.CloseEvent) {
		if closing {
			return
		}
		e.PreventDefault()
		p.Post(func() {
			app.RequestClose(func() {
				closing = true
				win.Close()
			})
		})
	})
	p.attach(win)
	win.Update(app.Start)
	win.Show()
	return win
}
