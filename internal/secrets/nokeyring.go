package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// noKeyring refuses secret operations without treating a token as revoked.
type noKeyring struct {
	cause error
	hint  string
}

var _ Store = (*noKeyring)(nil)

func (s *noKeyring) err(op, key string) error {
	var cause string
	if s.cause != nil {
		cause = s.cause.Error()
	}
	return fmt.Errorf("secrets: %s %s: %w", op, key, errNoKeyring(cause, s.hint))
}

func (s *noKeyring) Get(_ context.Context, key string) (string, error) {
	return "", s.err("get", key)
}

func (s *noKeyring) Set(_ context.Context, key, _ string) error {
	return s.err("set", key)
}

func (s *noKeyring) Delete(_ context.Context, key string) error {
	return s.err("delete", key)
}

func openNoKeyring(file string, cause error, opts Options, log *slog.Logger) Store {
	log.Error("no Secret Service: notesd cannot keep a device token, and pairing with a hub will fail", "err", cause, "hint", opts.Hint)
	if _, err := os.Stat(file); err == nil {
		log.Warn("the secrets file stays as it is until a Secret Service answers, then its tokens move in once", "file", file)
	}
	return &noKeyring{cause: cause, hint: opts.Hint}
}
