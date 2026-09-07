package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// testHome is the home directory every store in this file is built with. It is
// fictional on purpose: nothing here should touch the real one.
const testHome = "/home/tester"

// testDefaults is the fallback every store in this file falls back to.
func testDefaults() config.Config {
	return config.Default(testHome)
}

func TestStorePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	if got, want := config.NewStore(root, testHome).Path(), filepath.Join(root, config.FileName); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestStoreExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	exists, err := store.Exists()
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}

	if exists {
		t.Error("Exists() = true for an empty config root, want false")
	}

	if saveErr := store.Save(t.Context(), testDefaults()); saveErr != nil {
		t.Fatalf("Save() error = %v", saveErr)
	}

	exists, err = store.Exists()
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}

	if !exists {
		t.Error("Exists() = false after Save, want true")
	}
}

func TestStoreExistsOnAnUnreadableRoot(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores directory permissions")
	}

	root := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	exists, err := config.NewStore(filepath.Join(root, "nested"), testHome).Exists()
	if err == nil {
		t.Fatalf("Exists() = %t, nil, want an error for an unreadable root", exists)
	}
}

func TestStoreLoadCreatesTheDocumentWhenItIsMissing(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "cwm")
	store := config.NewStore(root, testHome)

	cfg, err := store.Load(t.Context())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg != testDefaults() {
		t.Errorf("Load() = %+v, want the defaults %+v", cfg, testDefaults())
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	want, encErr := testDefaults().Encode()
	if encErr != nil {
		t.Fatalf("Encode() error = %v", encErr)
	}

	if string(data) != string(want) {
		t.Errorf("document on disk = %q, want %q", data, want)
	}
}

func TestStoreLoadReadsAnExistingDocument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	// A document carrying a field this build does not know about must still
	// load: an older cwm has to survive a newer cwm's config.
	document := `{"schemaVersion": 1, "workspaceRoot": "/srv/workspaces", "unknownFuture": 1}`
	if err := os.WriteFile(store.Path(), []byte(document), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := store.Load(t.Context())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if want := "/srv/workspaces"; cfg.WorkspaceRoot != want {
		t.Errorf("Load().WorkspaceRoot = %q, want %q", cfg.WorkspaceRoot, want)
	}
}

func TestStoreLoadRejectsAMalformedDocument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	if err := os.WriteFile(store.Path(), []byte("not json"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := store.Load(t.Context())
	if err == nil {
		t.Fatalf("Load() = %+v, nil, want an error", cfg)
	}

	if !strings.Contains(err.Error(), store.Path()) {
		t.Errorf("Load() error = %q, want it to name %q", err, store.Path())
	}
}

func TestStoreLoadLeavesAnExistingDocumentAlone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	// Unknown fields are the tell: if Load rewrote the document from the
	// in-memory struct, this field would be gone.
	document := `{"schemaVersion": 1, "workspaceRoot": "/srv/workspaces"}`
	if err := os.WriteFile(store.Path(), []byte(document), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if _, err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != document {
		t.Errorf("document after Load = %q, want it untouched as %q", data, document)
	}
}

func TestStoreSavePermissionsAndLeftovers(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "cwm")
	store := config.NewStore(root, testHome)

	if err := store.Save(t.Context(), testDefaults()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	dir, err := os.Stat(root)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if mode := dir.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("config root mode = %#o, want no group or other access", mode)
	}

	file, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if mode := file.Mode().Perm(); mode != 0o600 {
		t.Errorf("document mode = %#o, want %#o", mode, 0o600)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != config.FileName {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		t.Errorf("config root holds %v, want only %q", names, config.FileName)
	}
}

func TestStoreSaveOverwrites(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	if err := os.WriteFile(store.Path(), []byte("stale, much longer than the real document"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := store.Save(t.Context(), testDefaults()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	want, encErr := testDefaults().Encode()
	if encErr != nil {
		t.Fatalf("Encode() error = %v", encErr)
	}

	if string(data) != string(want) {
		t.Errorf("document = %q, want %q", data, want)
	}
}

func TestStoreHonoursACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	if err := store.Save(ctx, testDefaults()); err == nil {
		t.Error("Save() error = nil, want a context error")
	}

	if _, err := store.Load(ctx); err == nil {
		t.Error("Load() error = nil, want a context error")
	}

	if _, err := os.Stat(store.Path()); err == nil {
		t.Error("a cancelled call still wrote the document")
	}
}

func TestStoreLoadFillsInBlankFieldsFromTheDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
	}{
		{"the document is empty", `{}`},
		{"the setting is empty", `{"workspaceRoot": ""}`},
		{"the setting is whitespace", `{"workspaceRoot": "   "}`},
		{"the schema version is absent", `{"workspaceRoot": "/home/tester/claude-workspaces"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			store := config.NewStore(root, testHome)

			if err := os.WriteFile(store.Path(), []byte(tt.document), 0o600); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}

			cfg, err := store.Load(t.Context())
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if cfg != testDefaults() {
				t.Errorf("Load() = %+v, want the defaults %+v", cfg, testDefaults())
			}
		})
	}
}

func TestStoreDefaults(t *testing.T) {
	t.Parallel()

	if got := config.NewStore(t.TempDir(), testHome).Defaults(); got != testDefaults() {
		t.Errorf("Defaults() = %+v, want %+v", got, testDefaults())
	}
}

func TestStoreReset(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := config.NewStore(root, testHome)

	if err := os.WriteFile(
		store.Path(),
		[]byte(`{"schemaVersion": 1, "workspaceRoot": "/srv/workspaces"}`),
		0o600,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := store.Reset(t.Context())
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if cfg != testDefaults() {
		t.Errorf("Reset() = %+v, want the defaults %+v", cfg, testDefaults())
	}

	reloaded, err := store.Load(t.Context())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if reloaded != testDefaults() {
		t.Errorf("the document after Reset = %+v, want the defaults %+v", reloaded, testDefaults())
	}
}

func TestStoreResetCreatesAMissingDocument(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "cwm")
	store := config.NewStore(root, testHome)

	if _, err := store.Reset(t.Context()); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	exists, err := store.Exists()
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}

	if !exists {
		t.Error("Exists() = false after Reset, want true")
	}
}

func TestStoreResetHonoursACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	store := config.NewStore(t.TempDir(), testHome)

	if _, err := store.Reset(ctx); err == nil {
		t.Error("Reset() error = nil, want a context error")
	}
}
