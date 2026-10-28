package packagecheck

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
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

// failureName is the name of the failure of generation gen, as the helper
// writes it.
func failureName(gen string) string {
	return "background-failure-" + gen + ".json"
}

// helperAnswer is the part of the helper's outcome that these tests read.
type helperAnswer struct {
	Generation *int    `json:"generation"`
	Error      *string `json:"error"`
}

// runHelper runs the request helper with XDG_STATE_HOME and XDG_CONFIG_HOME
// at fresh temporary directories, and with a lock it cannot take: the lock's
// path is a directory, so the helper's open of it fails. files are written
// in the app's state directory before the run. It answers that directory.
func runHelper(t *testing.T, files map[string]string) string {
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
	for name, content := range files {
		write(t, filepath.Join(dir, name), content)
	}
	cmd := exec.Command("python3", script)
	cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the helper did not exit 0: %v\n%s", err, out)
	}
	return dir
}

// readFile answers the content of the file name in dir, or fails the test.
func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// readAnswer decodes the outcome file name in dir.
func readAnswer(t *testing.T, dir, name string) helperAnswer {
	t.Helper()
	var answer helperAnswer
	if err := json.Unmarshal([]byte(readFile(t, dir, name)), &answer); err != nil {
		t.Fatalf("%s is not JSON: %v", name, err)
	}
	return answer
}

// checkFailure fails the test unless the failure name in dir has generation
// gen and an error.
func checkFailure(t *testing.T, dir, name string, gen int) {
	t.Helper()
	answer := readAnswer(t, dir, name)
	if answer.Generation == nil || *answer.Generation != gen {
		t.Errorf("%s has generation %v, want %d", name, answer.Generation, gen)
	}
	if answer.Error == nil {
		t.Errorf("%s has no error", name)
	}
}

// checkGone fails the test while the file name is in dir.
func checkGone(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Errorf("%s is still there: %v", name, err)
	}
}

// A helper that cannot take the lock never writes background.json, so a
// newer answer stays the one the app waits for. Its own failure goes beside
// that answer, with the counter it read.
func TestBackgroundHelperWritesAFailureBesideANewerAnswer(t *testing.T) {
	const answer = `{"generation": 2, "response": 0, "error": null}`
	dir := runHelper(t, map[string]string{counterName: "2\n", answerName: answer})
	if got := readFile(t, dir, answerName); got != answer {
		t.Errorf("the helper rewrote the answer:\n%s", got)
	}
	checkFailure(t, dir, failureName("2"), 2)
}

// An answer about an older setting does not answer the new one, and the
// helper still leaves it alone: the shell shows the newer failure instead.
func TestBackgroundHelperWritesAFailureBesideAnOlderAnswer(t *testing.T) {
	const answer = `{"generation": 1, "response": 2, "error": null}`
	dir := runHelper(t, map[string]string{counterName: "2\n", answerName: answer})
	if got := readFile(t, dir, answerName); got != answer {
		t.Errorf("the helper rewrote the answer:\n%s", got)
	}
	checkFailure(t, dir, failureName("2"), 2)
}

// The failures of the lower generations answer nothing that this helper's
// failure does not, so it removes them. A name that is not of their form is
// no failure, and stays.
func TestBackgroundHelperRemovesTheFailuresBelowItsGeneration(t *testing.T) {
	dir := runHelper(t, map[string]string{
		counterName:                 "3\n",
		failureName("1"):            `{"generation": 1, "error": "no lock"}`,
		failureName("2"):            `{"generation": 2, "error": "no lock"}`,
		"background-failure-x.json": `{"generation": 9, "error": "no lock"}`,
	})
	checkFailure(t, dir, failureName("3"), 3)
	checkGone(t, dir, failureName("1"))
	checkGone(t, dir, failureName("2"))
	checkGone(t, dir, answerName)
	if got := readFile(t, dir, "background-failure-x.json"); got == "" {
		t.Error("the helper removed a file that is no failure")
	}
}
