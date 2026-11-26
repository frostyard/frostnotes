package flatpakapp

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// tail is how much of notesd.log's end is read: a bounded read, whatever
// the log's size.
const tail = 8 * 1024

// PortalFailure starts the line the launcher appends when no notesd
// answers after flatpak-spawn. internal/packagecheck pins it to the
// launcher.
const PortalFailure = "frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn)"

// tailLines is how many of the log's last lines with text NotesdLog keeps.
const tailLines = 10

// NotesdLog is what notesd.log says of why notesd is not running
// (docs/design/app.md#the-flatpak).
type NotesdLog struct {
	Path string `json:"path"`
	// LastLine is the log's last line that holds text, "" for none.
	LastLine string `json:"lastLine"`
	// Lines are the log's last lines that hold text, at most 10, oldest
	// first; LastLine is the last of them.
	Lines []string `json:"lines"`
	// PortalRefused says that line is the launcher's: the Flatpak portal
	// did not start notesd.
	PortalRefused bool `json:"portalRefused"`
}

// ReadNotesdLog reports the end of notesd.log in the Flatpak; ok is false
// anywhere else, where a notesd a person runs has its own log.
func ReadNotesdLog(getenv Getenv) (log NotesdLog, ok bool) {
	if !InFlatpak(getenv) {
		return NotesdLog{}, false
	}
	d, err := StateDir(getenv)
	if err != nil {
		return NotesdLog{}, false
	}
	return describeLog(filepath.Join(d, "notesd.log")), true
}

func describeLog(path string) NotesdLog {
	lines := lastLines(readTail(path), tailLines)
	line := ""
	if len(lines) > 0 {
		line = lines[len(lines)-1]
	}
	return NotesdLog{Path: path, LastLine: line, Lines: lines, PortalRefused: strings.HasPrefix(line, PortalFailure)}
}

func readTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	if _, err := f.Seek(max(0, st.Size()-tail), io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, tail))
	if err != nil {
		return ""
	}
	return strings.ToValidUTF8(string(b), "�")
}

// lastLines are text's last n lines that hold text, trimmed, oldest first.
func lastLines(text string, n int) []string {
	lines := strings.Split(text, "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			out = append(out, t)
		}
	}
	slices.Reverse(out)
	return out
}
