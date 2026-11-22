package secrets

import (
	"context"
	"log/slog"
	"os"

	"github.com/godbus/dbus/v5"
)

// Open picks notesd's secret store: the Secret Service on the session bus,
// unless FROSTNOTES_SECRETS=file explicitly picks the file store. When no
// service answers, opts.NoFallback refuses secret operations; otherwise Open
// uses the file store with a warning. With the Secret Service, secrets still
// in the file are moved over and the file is removed.
func Open(ctx context.Context, file string, log *slog.Logger, opts Options) (Store, error) {
	fs := NewFile(file)
	if os.Getenv("FROSTNOTES_SECRETS") == "file" {
		return fs, nil
	}
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err == nil {
		var ss *SecretService
		if ss, err = OpenSecretService(ctx, conn); err == nil {
			ss.hint = opts.Hint
			if err := migrate(ctx, fs, ss); err != nil {
				return nil, err
			}
			return ss, nil
		}
		_ = conn.Close()
	}
	if opts.NoFallback {
		return openNoKeyring(file, err, opts, log), nil
	}
	log.Warn("no Secret Service; keeping secrets in a file", "file", file, "err", err)
	return fs, nil
}
