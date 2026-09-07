package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// stubEnv is a [paths.Environment] pinned to one directory, so the config
// commands can be exercised without touching the real user directories.
type stubEnv struct {
	configRoot string
	cacheRoot  string
}

func (s stubEnv) LookupEnv(key string) (string, bool) {
	switch key {
	case paths.ConfigRootEnv:
		return s.configRoot, true
	case paths.CacheRootEnv:
		return s.cacheRoot, true
	default:
		return "", false
	}
}

func (s stubEnv) UserConfigDir() (string, error) { return s.configRoot, nil }

func (s stubEnv) UserCacheDir() (string, error) { return s.cacheRoot, nil }

// runConfigCmd builds a root command rooted at a fresh temporary config root
// and runs args against it, returning the config root and the combined output.
func runConfigCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	root := t.TempDir()
	env := stubEnv{configRoot: filepath.Join(root, "config"), cacheRoot: filepath.Join(root, "cache")}

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{}, paths.New(env))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	// Execute first: return operands are evaluated before the call, so reading
	// the buffer in the return statement would capture it empty.
	err := cmd.Execute()

	return env.configRoot, out.String(), err
}

func TestConfigPathsReportsTheRoots(t *testing.T) {
	t.Parallel()

	configRoot, out, err := runConfigCmd(t, "config", "paths")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{configRoot, filepath.Join(configRoot, config.FileName), "not created yet"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestConfigPathsReportsAPresentDocument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := stubEnv{configRoot: filepath.Join(root, "config"), cacheRoot: filepath.Join(root, "cache")}

	if err := config.NewStore(env.configRoot).Save(t.Context(), config.Default()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{}, paths.New(env))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "paths"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "(present)") {
		t.Errorf("output missing %q, got:\n%s", "(present)", got)
	}
}

func TestConfigPathsReportsTheCacheRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := stubEnv{configRoot: filepath.Join(root, "config"), cacheRoot: filepath.Join(root, "cache")}

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{}, paths.New(env))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "paths"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, env.cacheRoot) {
		t.Errorf("output missing the cache root %q, got:\n%s", env.cacheRoot, got)
	}
}

func TestConfigShowCreatesTheDefaultDocument(t *testing.T) {
	t.Parallel()

	configRoot, out, err := runConfigCmd(t, "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if want := "{}\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	data, readErr := os.ReadFile(filepath.Join(configRoot, config.FileName))
	if readErr != nil {
		t.Fatalf("os.ReadFile() error = %v", readErr)
	}

	if want := "{}\n"; string(data) != want {
		t.Errorf("document on disk = %q, want %q", data, want)
	}
}

func TestConfigShowReportsAMalformedDocument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := stubEnv{configRoot: filepath.Join(root, "config"), cacheRoot: filepath.Join(root, "cache")}

	if err := os.MkdirAll(env.configRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	path := filepath.Join(env.configRoot, config.FileName)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{}, paths.New(env))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "show"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want an error for a malformed document")
	}

	if got := out.String(); !strings.Contains(got, path) {
		t.Errorf("error output missing %q, got:\n%s", path, got)
	}
}

func TestConfigGroupPrintsHelp(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"paths", "show", "Usage:"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q, got:\n%s", want, out)
		}
	}
}

func TestConfigSubcommandsRejectArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"paths takes no arguments", []string{"config", "paths", "unexpected"}},
		{"show takes no arguments", []string{"config", "show", "unexpected"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, _, err := runConfigCmd(t, tt.args...); err == nil {
				t.Error("Execute() error = nil, want an error for an unexpected argument")
			}
		})
	}
}
