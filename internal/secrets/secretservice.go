package secrets

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The Secret Service (org.freedesktop.secrets) on the session bus: GNOME
// Keyring, KWallet, KeePassXC (ADR-0016; frostmail ADR-0011).
// Items live in the default collection, which the service names with
// ReadAlias; when it has none, the store makes one, as libsecret does. Items
// are labeled "Frostnotes: <key>" with the attributes application=frostnotes
// and key=<key>; secrets travel in a plain session, since the bus is local.

const (
	ssName            = "org.freedesktop.secrets"
	ssPath            = dbus.ObjectPath("/org/freedesktop/secrets")
	ssService         = "org.freedesktop.Secret.Service"
	ssCollection      = "org.freedesktop.Secret.Collection"
	ssItem            = "org.freedesktop.Secret.Item"
	ssPrompt          = "org.freedesktop.Secret.Prompt"
	defaultCollection = dbus.ObjectPath("/org/freedesktop/secrets/aliases/default")
	noPrompt          = dbus.ObjectPath("/")
)

// ErrUnavailable means no Secret Service answered on the bus.
var ErrUnavailable = errors.New("secrets: no Secret Service")

// ErrDismissed means the user dismissed the service's unlock prompt.
var ErrDismissed = errors.New("secrets: the unlock prompt was dismissed")

// ErrNoKeyring means the Secret Service has no default collection and
// cannot make one, so no token can be kept in it.
var ErrNoKeyring = errors.New("secrets: the Secret Service cannot make a default collection: " +
	"set FROSTNOTES_SECRETS=file to keep tokens in a file")

// staleObjects are the D-Bus errors that say an object path is not there,
// which a session with no login keyring gives for the collection its
// default alias names.
var staleObjects = map[string]bool{
	"org.freedesktop.Secret.Error.NoSuchObject":   true,
	"org.freedesktop.DBus.Error.NoSuchObject":     true,
	"org.freedesktop.DBus.Error.UnknownObject":    true,
	"org.freedesktop.DBus.Error.UnknownMethod":    true,
	"org.freedesktop.DBus.Error.UnknownInterface": true,
}

// isStaleObject reports whether err says that an object path is gone.
func isStaleObject(err error) bool {
	var de dbus.Error
	if errors.As(err, &de) {
		return staleObjects[de.Name]
	}
	var pde *dbus.Error
	return errors.As(err, &pde) && staleObjects[pde.Name]
}

// secretStruct is the Secret Service's Secret type (oayays).
type secretStruct struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// SecretService is a Store in the desktop's Secret Service.
type SecretService struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
	mu      sync.Mutex // one prompt at a time
}

var _ Store = (*SecretService)(nil)

// OpenSecretService opens a plain session with the Secret Service on conn,
// or returns an error wrapping ErrUnavailable.
func OpenSecretService(ctx context.Context, conn *dbus.Conn) (*SecretService, error) {
	var out dbus.Variant
	var session dbus.ObjectPath
	err := conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".OpenSession", 0, "plain", dbus.MakeVariant("")).
		Store(&out, &session)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &SecretService{conn: conn, session: session}, nil
}

func attributes(key string) map[string]string {
	return map[string]string{"application": "frostnotes", "key": key}
}

// Get returns the secret stored under key, or ErrNotFound.
func (s *SecretService) Get(ctx context.Context, key string) (string, error) {
	item, err := s.find(ctx, key)
	if err != nil {
		return "", err
	}
	var sec secretStruct
	if err := s.conn.Object(ssName, item).CallWithContext(ctx, ssItem+".GetSecret", 0, s.session).Store(&sec); err != nil {
		return "", fmt.Errorf("secrets: get %s: %w", key, err)
	}
	return string(sec.Value), nil
}

// Set stores value under key, replacing any previous item.
func (s *SecretService) Set(ctx context.Context, key, value string) error {
	coll, err := s.collection(ctx)
	if err != nil {
		return fmt.Errorf("secrets: set %s: %w", key, err)
	}
	err = s.setIn(ctx, coll, key, value)
	if err != nil && isStaleObject(err) {
		// The collection the service named is not there: make the default
		// collection, as libsecret does, and try once more in it.
		made, cerr := s.createDefault(ctx)
		if cerr != nil {
			return fmt.Errorf("secrets: set %s: %w", key, cerr)
		}
		err = s.setIn(ctx, made, key, value)
	}
	if err != nil {
		return fmt.Errorf("secrets: set %s: %w", key, err)
	}
	return nil
}

// setIn unlocks the collection at coll and creates or replaces key's item
// in it, through the service's prompt when it asks for one.
func (s *SecretService) setIn(ctx context.Context, coll dbus.ObjectPath, key, value string) error {
	if err := s.unlock(ctx, []dbus.ObjectPath{coll}); err != nil {
		return err
	}
	props := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant("Frostnotes: " + key),
		ssItem + ".Attributes": dbus.MakeVariant(attributes(key)),
	}
	sec := secretStruct{Session: s.session, Value: []byte(value), ContentType: "text/plain; charset=utf8"}
	var item, prompt dbus.ObjectPath
	err := s.conn.Object(ssName, coll).CallWithContext(ctx, ssCollection+".CreateItem", 0, props, sec, true).
		Store(&item, &prompt)
	if err != nil {
		return err
	}
	if prompt != noPrompt {
		_, err := s.prompt(ctx, prompt)
		return err
	}
	return nil
}

// collection returns the path of the default collection, making the
// default collection when the service has none. A service without
// ReadAlias keeps its collection at the alias path.
func (s *SecretService) collection(ctx context.Context) (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".ReadAlias", 0, "default").Store(&path)
	if err != nil {
		return defaultCollection, nil
	}
	if path == noPrompt { // "/": no default collection
		return s.createDefault(ctx)
	}
	return path, nil
}

// createDefault makes the default collection, through the service's prompt
// when it asks for one, and returns its path.
func (s *SecretService) createDefault(ctx context.Context) (dbus.ObjectPath, error) {
	props := map[string]dbus.Variant{ssCollection + ".Label": dbus.MakeVariant("Default keyring")}
	var path, prompt dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".CreateCollection", 0, props, "default").
		Store(&path, &prompt)
	if err != nil {
		return "", fmt.Errorf("make the default collection: %v: %w", err, ErrNoKeyring)
	}
	if prompt != noPrompt {
		result, err := s.prompt(ctx, prompt)
		if err != nil {
			return "", err
		}
		made, ok := result.Value().(dbus.ObjectPath)
		if !ok || made == noPrompt {
			return "", fmt.Errorf("make the default collection: the prompt named no collection: %w", ErrNoKeyring)
		}
		return made, nil
	}
	if path == noPrompt {
		return "", fmt.Errorf("make the default collection: the service named no collection: %w", ErrNoKeyring)
	}
	return path, nil
}

// Delete removes key; a missing key is not an error.
func (s *SecretService) Delete(ctx context.Context, key string) error {
	item, err := s.find(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var prompt dbus.ObjectPath
	if err := s.conn.Object(ssName, item).CallWithContext(ctx, ssItem+".Delete", 0).Store(&prompt); err != nil {
		return fmt.Errorf("secrets: delete %s: %w", key, err)
	}
	if prompt != noPrompt {
		if _, err := s.prompt(ctx, prompt); err != nil {
			return fmt.Errorf("secrets: delete %s: %w", key, err)
		}
	}
	return nil
}

// find returns the item holding key, unlocking it when needed.
func (s *SecretService) find(ctx context.Context, key string) (dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".SearchItems", 0, attributes(key)).
		Store(&unlocked, &locked)
	if err != nil {
		return "", fmt.Errorf("secrets: search %s: %w", key, err)
	}
	if len(unlocked) > 0 {
		return unlocked[0], nil
	}
	if len(locked) == 0 {
		return "", fmt.Errorf("secrets: get %s: %w", key, ErrNotFound)
	}
	if err := s.unlock(ctx, locked[:1]); err != nil {
		return "", fmt.Errorf("secrets: get %s: %w", key, err)
	}
	return locked[0], nil
}

// unlock unlocks objects, through the service's prompt when it asks for one.
func (s *SecretService) unlock(ctx context.Context, objects []dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".Unlock", 0, objects).Store(&unlocked, &prompt)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if prompt == noPrompt {
		return nil
	}
	_, err = s.prompt(ctx, prompt)
	return err
}

// prompt shows a service prompt and waits for its Completed signal,
// returning the signal's result variant.
func (s *SecretService) prompt(ctx context.Context, path dbus.ObjectPath) (dbus.Variant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(ssPrompt), dbus.WithMatchMember("Completed")}
	if err := s.conn.AddMatchSignalContext(ctx, match...); err != nil {
		return dbus.Variant{}, fmt.Errorf("prompt: %w", err)
	}
	defer func() { _ = s.conn.RemoveMatchSignalContext(context.WithoutCancel(ctx), match...) }()
	signals := make(chan *dbus.Signal, 4)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)
	if err := s.conn.Object(ssName, path).CallWithContext(ctx, ssPrompt+".Prompt", 0, "").Err; err != nil {
		return dbus.Variant{}, fmt.Errorf("prompt: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return dbus.Variant{}, ctx.Err()
		case sig := <-signals:
			if sig.Path != path || sig.Name != ssPrompt+".Completed" || len(sig.Body) < 1 {
				continue
			}
			if dismissed, _ := sig.Body[0].(bool); dismissed {
				return dbus.Variant{}, ErrDismissed
			}
			if len(sig.Body) < 2 {
				return dbus.Variant{}, nil
			}
			result, _ := sig.Body[1].(dbus.Variant)
			return result, nil
		}
	}
}
