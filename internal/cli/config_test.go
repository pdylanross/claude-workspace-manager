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
	homeDir    string
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

func (s stubEnv) UserHomeDir() (string, error) { return s.homeDir, nil }

// newStubEnv returns an environment rooted at a fresh temporary directory, so
// the config commands never touch the real user directories.
func newStubEnv(t *testing.T) stubEnv {
	t.Helper()

	root := t.TempDir()

	return stubEnv{
		configRoot: filepath.Join(root, "config"),
		cacheRoot:  filepath.Join(root, "cache"),
		homeDir:    filepath.Join(root, "home"),
	}
}

// defaultsFor returns the configuration the commands fall back to under env.
func defaultsFor(env stubEnv) config.Config {
	return config.Default(env.homeDir)
}

// runCmd runs args against a fresh command tree built for env, with stdin
// supplied by input, and returns everything written to stdout and stderr.
func runCmd(t *testing.T, env stubEnv, input string, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{}, paths.New(env))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	// Execute first: return operands are evaluated before the call, so reading
	// the buffer in the return statement would capture it empty.
	err := cmd.Execute()

	return out.String(), err
}

// runConfigCmd is runCmd with no input, for the commands that ask nothing.
func runConfigCmd(t *testing.T, args ...string) (stubEnv, string, error) {
	t.Helper()

	env := newStubEnv(t)
	out, err := runCmd(t, env, "", args...)

	return env, out, err
}

func TestConfigPathsReportsTheRoots(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runCmd(t, env, "", "config", "paths")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"config root: ", env.configRoot, "cache root: ", env.cacheRoot} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}

	// The document is deliberately not reported here.
	if strings.Contains(out, config.FileName) {
		t.Errorf("output names %q, want only the roots, got:\n%s", config.FileName, out)
	}
}

func TestConfigShowCreatesTheDefaultDocument(t *testing.T) {
	t.Parallel()

	env, out, err := runConfigCmd(t, "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want, encErr := defaultsFor(env).Encode()
	if encErr != nil {
		t.Fatalf("Encode() error = %v", encErr)
	}

	if out != string(want) {
		t.Errorf("output = %q, want %q", out, want)
	}

	if !strings.Contains(out, filepath.Join(env.homeDir, config.WorkspaceDirName)) {
		t.Errorf("output missing the default workspace root, got:\n%s", out)
	}

	data, readErr := os.ReadFile(filepath.Join(env.configRoot, config.FileName))
	if readErr != nil {
		t.Fatalf("os.ReadFile() error = %v", readErr)
	}

	if string(data) != string(want) {
		t.Errorf("document on disk = %q, want %q", data, want)
	}
}

func TestConfigShowPrintsAnExistingDocument(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"workspaceRoot": "/srv/workspaces"}`)

	out, err := runCmd(t, env, "", "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "/srv/workspaces") {
		t.Errorf("output missing the configured workspace root, got:\n%s", out)
	}
}

func TestConfigShowReportsAMalformedDocument(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, "not json")

	out, err := runCmd(t, env, "", "config", "show")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for a malformed document")
	}

	path := filepath.Join(env.configRoot, config.FileName)
	if !strings.Contains(out, path) {
		t.Errorf("error output missing %q, got:\n%s", path, out)
	}
}

func TestConfigShowASingleSetting(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"workspaceRoot": "/srv/workspaces"}`)

	out, err := runCmd(t, env, "", "config", "show", "workspaceRoot")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Bare and unquoted, so a shell can use it directly.
	if want := "/srv/workspaces\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigShowAnUnknownSetting(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runCmd(t, env, "", "config", "show", "nope")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for an unknown setting")
	}

	for _, want := range []string{"nope", "known settings", "workspaceRoot"} {
		if !strings.Contains(out, want) {
			t.Errorf("error output missing %q, got:\n%s", want, out)
		}
	}
}

func TestConfigShowRejectsMoreThanOneSetting(t *testing.T) {
	t.Parallel()

	if _, _, err := runConfigCmd(t, "config", "show", "workspaceRoot", "andAnother"); err == nil {
		t.Error("Execute() error = nil, want an error for a second argument")
	}
}

func TestConfigSet(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runCmd(t, env, "", "config", "set", "workspaceRoot=/srv/workspaces")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "/srv/workspaces") {
		t.Errorf("output missing the new value, got:\n%s", out)
	}

	if got := readDocument(t, env); !strings.Contains(got, "/srv/workspaces") {
		t.Errorf("document on disk = %s, want it to hold the new value", got)
	}
}

func TestConfigSetTakesSeveralAssignments(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	// Both name the same setting today, because the document has one setting;
	// what matters is that every argument is applied, last one winning.
	if _, err := runCmd(t, env, "", "config", "set", "workspaceRoot=/first", "workspaceRoot=/second"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readDocument(t, env); !strings.Contains(got, "/second") {
		t.Errorf("document on disk = %s, want the last assignment to win", got)
	}
}

func TestConfigSetValuesMayContainSeparators(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	// Only the first "=" separates; the rest belongs to the value.
	if _, err := runCmd(t, env, "", "config", "set", "workspaceRoot=/srv/a=b"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readDocument(t, env); !strings.Contains(got, "/srv/a=b") {
		t.Errorf("document on disk = %s, want the whole value kept", got)
	}
}

func TestConfigSetClearingASettingRestoresItsDefault(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"workspaceRoot": "/srv/workspaces"}`)

	out, err := runCmd(t, env, "", "config", "set", "workspaceRoot=")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := filepath.Join(env.homeDir, config.WorkspaceDirName)
	if !strings.Contains(out, want) {
		t.Errorf("output missing the restored default %q, got:\n%s", want, out)
	}

	if got := readDocument(t, env); !strings.Contains(got, want) {
		t.Errorf("document on disk = %s, want the default %q", got, want)
	}
}

func TestConfigSetRejectsBadArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arg      string
		wantText string
	}{
		{"no separator at all", "workspaceRoot", "SETTING=VALUE"},
		{"no setting before the separator", "=/srv/workspaces", "does not name a setting"},
		{"an unknown setting", "nope=1", "known settings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newStubEnv(t)

			out, err := runCmd(t, env, "", "config", "set", tt.arg)
			if err == nil {
				t.Fatalf("Execute() error = nil, want an error for %q", tt.arg)
			}

			if !strings.Contains(out, tt.wantText) {
				t.Errorf("error output missing %q, got:\n%s", tt.wantText, out)
			}
		})
	}
}

func TestConfigSetWritesNothingWhenAnAssignmentFails(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	document := `{"workspaceRoot": "/srv/workspaces"}`
	writeDocument(t, env, document)

	// The first assignment is good and the second is not; neither may land.
	if _, err := runCmd(t, env, "", "config", "set", "workspaceRoot=/changed", "nope=1"); err == nil {
		t.Fatal("Execute() error = nil, want an error")
	}

	if got := readDocument(t, env); got != document {
		t.Errorf("document on disk = %q, want it untouched as %q", got, document)
	}
}

func TestConfigSetNeedsAnArgument(t *testing.T) {
	t.Parallel()

	if _, _, err := runConfigCmd(t, "config", "set"); err == nil {
		t.Error("Execute() error = nil, want an error when nothing is assigned")
	}
}

func TestConfigResetDiscardsTheDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		args  []string
	}{
		{"confirmed with y", "y\n", []string{"config", "reset"}},
		{"confirmed with yes", "YES\n", []string{"config", "reset"}},
		{"confirmation skipped with --yes", "", []string{"config", "reset", "--yes"}},
		{"confirmation skipped with -y", "", []string{"config", "reset", "-y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newStubEnv(t)
			writeDocument(t, env, `{"workspaceRoot": "/srv/workspaces"}`)

			out, err := runCmd(t, env, tt.input, tt.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if strings.Contains(out, "/srv/workspaces") {
				t.Errorf("output still shows the discarded workspace root, got:\n%s", out)
			}

			want, encErr := defaultsFor(env).Encode()
			if encErr != nil {
				t.Fatalf("Encode() error = %v", encErr)
			}

			data, readErr := os.ReadFile(filepath.Join(env.configRoot, config.FileName))
			if readErr != nil {
				t.Fatalf("os.ReadFile() error = %v", readErr)
			}

			if string(data) != string(want) {
				t.Errorf("document on disk = %q, want the defaults %q", data, want)
			}
		})
	}
}

func TestConfigResetDeclined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"answered no", "n\n"},
		{"answered with an empty line", "\n"},
		{"answered with something else", "maybe\n"},
		{"nothing on the input at all", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newStubEnv(t)
			document := `{"workspaceRoot": "/srv/workspaces"}`
			writeDocument(t, env, document)

			out, err := runCmd(t, env, tt.input, "config", "reset")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if !strings.Contains(out, "nothing was written") {
				t.Errorf("output missing the cancellation notice, got:\n%s", out)
			}

			data, readErr := os.ReadFile(filepath.Join(env.configRoot, config.FileName))
			if readErr != nil {
				t.Fatalf("os.ReadFile() error = %v", readErr)
			}

			if string(data) != document {
				t.Errorf("document on disk = %q, want it untouched as %q", data, document)
			}
		})
	}
}

func TestConfigResetAsksBeforeWriting(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runCmd(t, env, "y\n", "config", "reset")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	path := filepath.Join(env.configRoot, config.FileName)
	if !strings.Contains(out, path) {
		t.Errorf("prompt does not name %q, got:\n%s", path, out)
	}
}

func TestConfigGroupPrintsHelp(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"paths", "show", "set", "reset", "Usage:"} {
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
		{"reset takes no arguments", []string{"config", "reset", "-y", "unexpected"}},
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

// writeDocument puts document at the config document's path under env.
func writeDocument(t *testing.T, env stubEnv, document string) {
	t.Helper()

	if err := os.MkdirAll(env.configRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	path := filepath.Join(env.configRoot, config.FileName)
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}

// readDocument returns the config document under env.
func readDocument(t *testing.T, env stubEnv) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(env.configRoot, config.FileName))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	return string(data)
}
