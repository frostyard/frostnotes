package packagecheck

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// backgroundHelper is the launcher's request helper, as the Flatpak ships it.
const backgroundHelper = "../../packaging/flatpak/request-background.py"

// The names the helper uses in the app's state directory.
const (
	answerName  = "background.json"
	counterName = "background-gen"
	lockName    = "background.lock"
)

// helperAnswer is the part of the helper's outcome that these tests read.
type helperAnswer struct {
	Generation *int    `json:"generation"`
	Error      *string `json:"error"`
}

// runHelper runs the request helper with XDG_STATE_HOME and XDG_CONFIG_HOME
// at fresh temporary directories, and with a lock it cannot take: the lock's
// path is a directory, so the helper's open of it fails. counter is the
// counter file's content, or "" for no file; answer is the outcome file's
// content, or "" for no file. It returns the outcome file's path and the
// helper's output.
func runHelper(t *testing.T, counter, answer string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	script, err := filepath.Abs(backgroundHelper)
	if err != nil {
		t.Fatal(err)
	}
	state, config := t.TempDir(), t.TempDir()
	dir := filepath.Join(state, "frostnotes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, lockName), 0o755); err != nil {
		t.Fatal(err)
	}
	if counter != "" {
		write(t, filepath.Join(dir, counterName), counter)
	}
	path := filepath.Join(dir, answerName)
	if answer != "" {
		write(t, path, answer)
	}
	cmd := exec.Command("python3", script)
	cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the helper did not exit 0: %v\n%s", err, out)
	}
	return path, string(out)
}

// readAnswer decodes the outcome file the helper left.
func readAnswer(t *testing.T, path string) helperAnswer {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var answer helperAnswer
	if err := json.Unmarshal(data, &answer); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", path, err, data)
	}
	return answer
}

// A helper that cannot take the lock must not replace an answer whose
// generation is at least the counter it read: the app waits for such an
// answer, and an older failure would leave it on "Asking the desktop…".
func TestBackgroundHelperKeepsANewerAnswer(t *testing.T) {
	const answer = `{"generation": 2, "response": 0, "error": null}`
	path, out := runHelper(t, "2\n", answer)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != answer {
		t.Errorf("the helper rewrote the newer answer:\n%s", got)
	}
	if !strings.Contains(out, "keeps the newer answer") {
		t.Errorf("the helper does not log that it kept the answer:\n%s", out)
	}
}

// An answer about an older setting stays no answer to the new one, so the
// failure replaces it, with the counter read at the write.
func TestBackgroundHelperReplacesAnOlderAnswer(t *testing.T) {
	path, _ := runHelper(t, "2\n", `{"generation": 1, "response": 2, "error": null}`)
	answer := readAnswer(t, path)
	if answer.Generation == nil || *answer.Generation != 2 {
		t.Errorf("generation is not 2: %v", answer.Generation)
	}
	if answer.Error == nil {
		t.Error("the answer has no error")
	}
}

func TestBackgroundHelperWritesAFailureWithoutAnAnswer(t *testing.T) {
	path, _ := runHelper(t, "3\n", "")
	answer := readAnswer(t, path)
	if answer.Generation == nil || *answer.Generation != 3 {
		t.Errorf("generation is not 3: %v", answer.Generation)
	}
	if answer.Error == nil {
		t.Error("the answer has no error")
	}
}
