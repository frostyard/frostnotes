package system

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// tail is how much of notesd.log's end is read: a bounded read, whatever
// the log's size.
const tail = 8 * 1024

// PortalFailure starts the line the launcher appends when no notesd
// answers after flatpak-spawn. internal/packagecheck pins it to the
// launcher.
const PortalFailure = "frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn)"

// NotesdLog is what the window learns from notesd.log: why notesd is not
// running (docs/design/app.md#the-flatpak).
type NotesdLog struct {
	Path string
	// LastLine is the log's last line that holds text, "" for none.
	LastLine string
	// PortalRefused says that line is the launcher's: the Flatpak portal
	// did not start notesd.
	PortalRefused bool
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
	line := lastLine(readTail(path))
	return NotesdLog{Path: path, LastLine: line, PortalRefused: strings.HasPrefix(line, PortalFailure)}
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

func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}
