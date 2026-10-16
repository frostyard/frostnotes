package main

import (
	"fmt"
	"strconv"
	"strings"
)

// An executor is the coding agent taskrun hands a card to
// (docs/design/agent-workflow.md): Codex with the user's model
// (ADR-0010), or opencode with a local model.
type executor struct {
	name  string // "codex" or "opencode"
	model string // the model to use; "" leaves Codex on the user's default
	root  string // the repository
	title string // the session's title, for opencode
	// writable are the directories outside the repository that Codex's
	// sandbox lets commands write: Go's build cache, golangci-lint's and
	// mise's caches.
	writable []string
}

// command returns the program and arguments for a session's first prompt
// (resume false) or for a follow-up in the same session (resume true).
func (e executor) command(prompt string, resume bool) (string, []string, error) {
	switch e.name {
	case "codex":
		return "codex", e.codexArgs(prompt, resume), nil
	case "opencode":
		if e.model == "" {
			return "", nil, fmt.Errorf("opencode needs a model: pass -model or set FROSTNOTES_EXECUTOR_MODEL")
		}
		if resume {
			return "opencode", []string{"run", "--continue", "--model", e.model, prompt}, nil
		}
		return "opencode", []string{"run", "--model", e.model, "--title", e.title, prompt}, nil
	}
	return "", nil, fmt.Errorf("unknown executor %q (codex or opencode)", e.name)
}

// codexArgs runs Codex non-interactively in a workspace-write sandbox:
// edits stay in the repository, and commands may write the repository,
// /tmp and the caches in writable, and use the network (the sandbox
// refuses even loopback sockets without it, and tests start servers). A
// follow-up resumes the newest session started in the repository, which is
// the card's own: taskrun runs one card at a time.
func (e executor) codexArgs(prompt string, resume bool) []string {
	quoted := make([]string, len(e.writable))
	for i, d := range e.writable {
		quoted[i] = strconv.Quote(d)
	}
	cfg := []string{
		"-c", `sandbox_mode="workspace-write"`,
		"-c", "sandbox_workspace_write.writable_roots=[" + strings.Join(quoted, ", ") + "]",
		"-c", "sandbox_workspace_write.network_access=true",
		"-c", `approval_policy="never"`,
	}
	if e.model != "" {
		cfg = append(cfg, "-c", "model="+strconv.Quote(e.model))
	}
	if resume {
		return append(append([]string{"exec", "resume", "--last"}, cfg...), prompt)
	}
	return append(append([]string{"exec", "--color", "never", "-C", e.root}, cfg...), prompt)
}

// guidance is what the executor is told besides the card: how to run the
// gates from its shell.
func (e executor) guidance(c *Card) string {
	if e.name != "codex" {
		return ""
	}
	s := "Your shell does not load the repository's pinned tools: run make through mise, as in " +
		"`mise exec -- make accept T=" + c.ID + "` and `mise exec -- make check`."
	if TouchesApp(c) {
		s += " The app's make targets (`make ui-check`, `make ui-vitest`, `make ui-fmt`) run in the nsl machine, " +
			"which your sandbox cannot reach: do not run them. Run the same tools on the host instead, from app/: " +
			"`mise exec -- pnpm exec vitest run <test files>`, `mise exec -- pnpm exec biome check --write <your files>` " +
			"and `mise exec -- pnpm exec tsc --noEmit`; never `pnpm install`. taskrun formats your files and " +
			"runs the make targets after you finish, and sends you any failure."
	}
	return s + "\n"
}
