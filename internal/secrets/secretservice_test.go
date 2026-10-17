package secrets

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostnotes/internal/dbustest"
	"github.com/godbus/dbus/v5"
)

func privateBus(t *testing.T) string { return dbustest.Bus(t) }

func connect(t *testing.T, addr string) *dbus.Conn { return dbustest.Connect(t, addr) }

// fakeSecrets is a Secret Service with one collection, enough for notesd.
type fakeSecrets struct {
	conn *dbus.Conn

	mu      sync.Mutex
	items   map[dbus.ObjectPath]*fakeItem
	next    int
	locked  bool
	dismiss bool
	prompts int
}

type fakeItem struct {
	attrs  map[string]string
	label  string
	value  []byte
	locked bool
}

func startFake(t *testing.T, addr string) *fakeSecrets {
	t.Helper()
	conn := connect(t, addr)
	f := &fakeSecrets{conn: conn, items: map[dbus.ObjectPath]*fakeItem{}}
	if err := conn.Export(fakeService{f}, ssPath, ssService); err != nil {
		t.Fatal(err)
	}
	if err := conn.Export(fakeCollection{f}, defaultCollection, ssCollection); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.RequestName(ssName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own %s: %v %v", ssName, reply, err)
	}
	return f
}

// lockAll locks the collection and every item.
func (f *fakeSecrets) lockAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked = true
	for _, it := range f.items {
		it.locked = true
	}
}

func (f *fakeSecrets) snapshot() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, it := range f.items {
		out[it.attrs["key"]] = string(it.value)
	}
	return out
}

type fakeService struct{ f *fakeSecrets }

func (s fakeService) OpenSession(algorithm string, _ dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	if algorithm != "plain" {
		return dbus.Variant{}, "", dbus.MakeFailedError(errors.New("unsupported algorithm"))
	}
	return dbus.MakeVariant(""), "/org/freedesktop/secrets/session/1", nil
}

func (s fakeService) SearchItems(attrs map[string]string) ([]dbus.ObjectPath, []dbus.ObjectPath, *dbus.Error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	unlocked, locked := []dbus.ObjectPath{}, []dbus.ObjectPath{}
	for p, it := range s.f.items {
		match := true
		for k, v := range attrs {
			if it.attrs[k] != v {
				match = false
			}
		}
		switch {
		case !match:
		case it.locked:
			locked = append(locked, p)
		default:
			unlocked = append(unlocked, p)
		}
	}
	return unlocked, locked, nil
}

func (s fakeService) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	needs := s.f.locked
	for _, o := range objects {
		if it := s.f.items[o]; it != nil && it.locked {
			needs = true
		}
	}
	if !needs {
		return objects, noPrompt, nil
	}
	s.f.prompts++
	path := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/secrets/prompt/%d", s.f.prompts))
	if err := s.f.conn.Export(fakePrompt{s.f, path}, path, ssPrompt); err != nil {
		return nil, "", dbus.MakeFailedError(err)
	}
	return []dbus.ObjectPath{}, path, nil
}

type fakePrompt struct {
	f    *fakeSecrets
	path dbus.ObjectPath
}

func (p fakePrompt) Prompt(string) *dbus.Error {
	p.f.mu.Lock()
	dismiss := p.f.dismiss
	if !dismiss {
		p.f.locked = false
		for _, it := range p.f.items {
			it.locked = false
		}
	}
	p.f.mu.Unlock()
	go func() {
		_ = p.f.conn.Emit(p.path, ssPrompt+".Completed", dismiss, dbus.MakeVariant(""))
	}()
	return nil
}

type fakeCollection struct{ f *fakeSecrets }

func (c fakeCollection) CreateItem(props map[string]dbus.Variant, secret secretStruct, replace bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if c.f.locked {
		return "", "", &dbus.Error{Name: "org.freedesktop.Secret.Error.IsLocked"}
	}
	attrs, _ := props[ssItem+".Attributes"].Value().(map[string]string)
	label, _ := props[ssItem+".Label"].Value().(string)
	if replace {
		for p, it := range c.f.items {
			if maps.Equal(it.attrs, attrs) {
				it.value, it.label = secret.Value, label
				return p, noPrompt, nil
			}
		}
	}
	c.f.next++
	path := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/secrets/collection/login/%d", c.f.next))
	c.f.items[path] = &fakeItem{attrs: attrs, label: label, value: secret.Value}
	if err := c.f.conn.Export(fakeItemObject{c.f, path}, path, ssItem); err != nil {
		return "", "", dbus.MakeFailedError(err)
	}
	return path, noPrompt, nil
}

type fakeItemObject struct {
	f    *fakeSecrets
	path dbus.ObjectPath
}

func (o fakeItemObject) GetSecret(session dbus.ObjectPath) (secretStruct, *dbus.Error) {
	o.f.mu.Lock()
	defer o.f.mu.Unlock()
	it := o.f.items[o.path]
	switch {
	case it == nil:
		return secretStruct{}, &dbus.Error{Name: "org.freedesktop.Secret.Error.NoSuchObject"}
	case it.locked:
		return secretStruct{}, &dbus.Error{Name: "org.freedesktop.Secret.Error.IsLocked"}
	}
	return secretStruct{Session: session, Value: it.value, ContentType: "text/plain"}, nil
}

func (o fakeItemObject) Delete() (dbus.ObjectPath, *dbus.Error) {
	o.f.mu.Lock()
	defer o.f.mu.Unlock()
	delete(o.f.items, o.path)
	return noPrompt, nil
}

func openFake(t *testing.T) (*SecretService, *fakeSecrets) {
	t.Helper()
	addr := privateBus(t)
	f := startFake(t, addr)
	ss, err := OpenSecretService(t.Context(), connect(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	return ss, f
}

func TestSecretServiceStore(t *testing.T) {
	ss, f := openFake(t)
	ctx := t.Context()
	if _, err := ss.Get(ctx, HubToken("one")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing = %v", err)
	}
	if err := ss.Set(ctx, HubToken("one"), "first"); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, HubToken("one"), "second"); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, HubToken("two"), "other"); err != nil {
		t.Fatal(err)
	}
	if got, err := ss.Get(ctx, HubToken("one")); err != nil || got != "second" {
		t.Fatalf("get = %q, %v", got, err)
	}
	if got := f.snapshot(); len(got) != 2 {
		t.Fatalf("items = %v; replacing must not add one", got)
	}
	f.mu.Lock() // the bus's goroutines change items
	for _, it := range f.items {
		if !strings.HasPrefix(it.label, "Frostnotes: hub/") || it.attrs["application"] != "frostnotes" {
			t.Errorf("item %+v", it)
		}
	}
	f.mu.Unlock()
	if err := ss.Delete(ctx, HubToken("one")); err != nil {
		t.Fatal(err)
	}
	if err := ss.Delete(ctx, HubToken("one")); err != nil {
		t.Fatalf("deleting a missing key: %v", err)
	}
	if _, err := ss.Get(ctx, HubToken("one")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted = %v", err)
	}
}

func TestSecretServiceUnlocksThroughThePrompt(t *testing.T) {
	ss, f := openFake(t)
	ctx := t.Context()
	if err := ss.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	f.lockAll()
	if got, err := ss.Get(ctx, "k"); err != nil || got != "v" {
		t.Fatalf("get locked = %q, %v", got, err)
	}
	f.lockAll()
	if err := ss.Set(ctx, "k2", "v2"); err != nil {
		t.Fatalf("set in a locked collection: %v", err)
	}
	f.lockAll()
	f.mu.Lock()
	f.dismiss = true
	f.mu.Unlock()
	if _, err := ss.Get(ctx, "k"); !errors.Is(err, ErrDismissed) {
		t.Fatalf("dismissed = %v", err)
	}
}

func TestOpenPicksAndMigrates(t *testing.T) {
	addr := privateBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	t.Setenv("FROSTNOTES_SECRETS", "")
	log := slog.New(slog.DiscardHandler)
	file := filepath.Join(t.TempDir(), "secrets.json")
	old := NewFile(file)
	if err := old.Set(t.Context(), HubToken("one"), "pw"); err != nil {
		t.Fatal(err)
	}

	// No service on the bus: the file store, untouched.
	st, err := Open(t.Context(), file, log)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*File); !ok {
		t.Fatalf("store = %T, want *File", st)
	}

	f := startFake(t, addr)
	st, err = Open(t.Context(), file, log)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*SecretService); !ok {
		t.Fatalf("store = %T, want *SecretService", st)
	}
	if got := f.snapshot(); got[HubToken("one")] != "pw" {
		t.Errorf("migrated = %v", got)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the file store is still there: %v", err)
	}
	if v, err := st.Get(t.Context(), HubToken("one")); err != nil || v != "pw" {
		t.Errorf("get after migration = %q, %v", v, err)
	}

	t.Setenv("FROSTNOTES_SECRETS", "file")
	st, err = Open(t.Context(), file, log)
	if _, ok := st.(*File); err != nil || !ok {
		t.Errorf("FROSTNOTES_SECRETS=file gave %T, %v", st, err)
	}
}
