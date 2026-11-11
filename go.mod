module github.com/frostyard/frostnotes

go 1.27.4

require (
	github.com/egoist/mygo v0.9.0
	github.com/frostyard/clix v0.5.0
	github.com/frostyard/frostui v0.3.0
	github.com/frostyard/lodge/kit v0.4.0
	github.com/frostyard/std v0.5.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gen2brain/gav1d v0.2.6
	github.com/godbus/dbus/v5 v5.2.4
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/spf13/cobra v1.10.2
	github.com/yuin/goldmark v1.8.7
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/net v0.62.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	modernc.org/sqlite v1.61.1
)

require (
	charm.land/fang/v2 v2.0.1 // indirect
	charm.land/lipgloss/v2 v2.0.6 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260811164956-006e29f97886 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/exp/charmtone v0.0.0-20260323091123-df7b1bcffcca // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.28 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/mango v0.2.0 // indirect
	github.com/muesli/mango-cobra v1.3.0 // indirect
	github.com/muesli/mango-pflag v0.2.0 // indirect
	github.com/muesli/roff v0.1.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// The native desktop app needs egoist/mygo#1662 (elements between lines),
// which no MyGo release has yet. This is branch bk/editor-blocks-89d18f on a
// fork: v0.9.0 and #1662, nothing else. The fork has no v0.9.0 tag, so its
// pseudo-version reads v0.8.4. Once #1662 is in a MyGo release, require that
// release and drop this (docs/plans/0009-mygo-desktop.md). While it stays,
// make verify also runs frostui's tests against it; the step and the replace
// go together.
replace github.com/egoist/mygo => github.com/bketelsen/mygo v0.8.4-0.20261111112850-24c98d3547ea
