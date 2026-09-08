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

// document renders a config document holding workspaceRoot, at the current
// schema, the way cwm would have written it.
func document(workspaceRoot string) string {
	return `{"schemaVersion": 1, "workspaceRoot": "` + workspaceRoot + `"}`
}

// runCmd runs args against a fresh command tree built for env, with stdin
// supplied by input, and returns everything written to stdout and stderr.
func runCmd(t *testing.T, env stubEnv, input string, args ...string) (string, error) {
	t.Helper()

	return runCmdWith(t, env, &stubUpdater{updatable: false, due: false}, input, args...)
}

// runCmdWith is runCmd against a specific updater.
func runCmdWith(t *testing.T, env stubEnv, updater *stubUpdater, input string, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := cli.NewRootCmd(
		version.Info{Version: "1.2.0", Commit: "", Date: ""},
		paths.New(env),
		updater.factory(),
		stubToolFactory(),
	)
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

	if !strings.Contains(out, filepath.Join(env.homeDir, config.WorkspaceDirName)) {
		t.Errorf("output missing the default workspace root, got:\n%s", out)
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
		t.Errorf("document on disk = %q, want %q", data, want)
	}
}

func TestConfigShowPrintsAnExistingDocument(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, document("/srv/workspaces"))

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
	writeDocument(t, env, document("/srv/workspaces"))

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
	writeDocument(t, env, document("/srv/workspaces"))

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
	existing := document("/srv/workspaces")
	writeDocument(t, env, existing)

	// The first assignment is good and the second is not; neither may land.
	if _, err := runCmd(t, env, "", "config", "set", "workspaceRoot=/changed", "nope=1"); err == nil {
		t.Fatal("Execute() error = nil, want an error")
	}

	if got := readDocument(t, env); got != existing {
		t.Errorf("document on disk = %q, want it untouched as %q", got, existing)
	}
}

func TestConfigSetNeedsAnArgument(t *testing.T) {
	t.Parallel()

	if _, _, err := runConfigCmd(t, "config", "set"); err == nil {
		t.Error("Execute() error = nil, want an error when nothing is assigned")
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

func TestConfigSetExpandsALeadingTilde(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	// zsh does not expand the tilde in "workspaceRoot=~/ws", so cwm has to.
	out, err := runCmd(t, env, "", "config", "set", "workspaceRoot=~/ws")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := filepath.Join(env.homeDir, "ws")
	if !strings.Contains(out, want) {
		t.Errorf("output missing the expanded path %q, got:\n%s", want, out)
	}

	if got := readDocument(t, env); !strings.Contains(got, want) {
		t.Errorf("document on disk = %s, want the expanded path %q", got, want)
	}

	if strings.Contains(readDocument(t, env), "~") {
		t.Errorf("document on disk still holds a tilde: %s", readDocument(t, env))
	}
}

func TestConfigSetRejectsARelativePath(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	existing := document("/srv/workspaces")
	writeDocument(t, env, existing)

	out, err := runCmd(t, env, "", "config", "set", "workspaceRoot=relative/path")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for a relative path")
	}

	if !strings.Contains(out, "absolute path") {
		t.Errorf("error output missing the reason, got:\n%s", out)
	}

	if got := readDocument(t, env); got != existing {
		t.Errorf("document on disk = %q, want it untouched as %q", got, existing)
	}
}

func TestConfigShowRejectsADocumentFromANewerCwm(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion": 99, "workspaceRoot": "/srv/workspaces"}`)

	out, err := runCmd(t, env, "", "config", "show")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for a newer schema")
	}

	if !strings.Contains(out, "upgrade cwm") {
		t.Errorf("error output missing the advice to upgrade, got:\n%s", out)
	}

	// Wiping the settings is the wrong fix for a document cwm is too old for.
	if strings.Contains(out, "config reset") {
		t.Errorf("error output suggests resetting, got:\n%s", out)
	}
}

func TestConfigResetDiscardsTheDocument(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, document("/srv/workspaces"))

	out, err := runCmd(t, env, "", "config", "reset", "--yes")
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

	if got := readDocument(t, env); got != string(want) {
		t.Errorf("document on disk = %q, want the defaults %q", got, want)
	}
}

func TestConfigResetRescuesABrokenDocument(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion": 1, "workspaceRoot": "relative"}`)

	// Load refuses the document, so reset has to work without reading it.
	out, err := runCmd(t, env, "", "config", "reset", "-y")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := filepath.Join(env.homeDir, config.WorkspaceDirName)
	if !strings.Contains(out, want) {
		t.Errorf("output missing the default workspace root %q, got:\n%s", want, out)
	}
}

func TestConfigShowRejectsSchemaVersionAsASetting(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runCmd(t, env, "", "config", "show", "schemaVersion")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for a field cwm keeps for itself")
	}

	if !strings.Contains(out, "keeps for itself") {
		t.Errorf("error output missing the reason, got:\n%s", out)
	}
}

func TestConfigShowIncludesTheSchemaVersion(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config", "show", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Not addressable, but still visible in the document it belongs to.
	if !strings.Contains(out, `"schemaVersion"`) {
		t.Errorf("output missing the schema version, got:\n%s", out)
	}
}

func TestConfigShowIsFormattedByDefault(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Formatted for reading, not the document.
	if strings.Contains(out, "{") || strings.Contains(out, `"workspaceRoot"`) {
		t.Errorf("output looks like JSON, want a formatted listing, got:\n%s", out)
	}

	for _, want := range []string{"workspaceRoot", "update", "mode", "auto", "forge", "kind", "none"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestConfigShowIsNotStyledWhenPipedIntoABuffer(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Styles are built from a renderer bound to the command's writer, so this
	// holds whether or not the test itself is run from a terminal.
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output carries ANSI escapes, got:\n%q", out)
	}
}

func TestConfigShowHidesTheOtherForge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string
		shown   string
		hidden  string
		visible string
	}{
		{"github hides gitlab", "github", "github", "gitlab", "forge.github.space"},
		{"gitlab hides github", "gitlab", "gitlab", "github", "forge.gitlab.space"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newStubEnv(t)

			if _, err := runCmd(t, env, "", "config", "set", "forge.kind="+tt.kind); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			out, err := runCmd(t, env, "", "config", "show")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			// The heading line, not the bare word: a temporary directory path
			// can easily contain "github" and say nothing about the listing.
			if !strings.Contains(out, "\n  "+tt.shown+"\n") {
				t.Errorf("output missing the %q group, got:\n%s", tt.shown, out)
			}

			if strings.Contains(out, "\n  "+tt.hidden+"\n") {
				t.Errorf("output shows the %q group, which does nothing here:\n%s", tt.hidden, out)
			}

			// The document still holds both; only the listing is filtered.
			raw, err := runCmd(t, env, "", "config", "show", "--json")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if !strings.Contains(raw, `"`+tt.hidden+`"`) {
				t.Errorf("--json is missing %q, want the document as it is on disk:\n%s", tt.hidden, raw)
			}
		})
	}
}

func TestConfigShowHidesBothForgesUntilOneIsChosen(t *testing.T) {
	t.Parallel()

	_, out, err := runConfigCmd(t, "config", "show")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, hidden := range []string{"github", "gitlab"} {
		if strings.Contains(out, "\n  "+hidden+"\n") {
			t.Errorf("output shows the %q group before a forge is chosen, got:\n%s", hidden, out)
		}
	}
}

func TestEverySettingIsVisibleUnderSomeForge(t *testing.T) {
	t.Parallel()

	// A guard against a setting that is hidden in every configuration, which
	// is how a new setting would silently never appear. The union of what is
	// shown across the forge kinds has to be everything.
	seen := map[string]bool{}

	for _, kind := range []string{"none", "github", "gitlab"} {
		env := newStubEnv(t)

		if _, err := runCmd(t, env, "", "config", "set", "forge.kind="+kind); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		out, err := runCmd(t, env, "", "config", "show")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		for _, setting := range config.Settings() {
			_, leaf := lastSegment(setting)
			if strings.Contains(out, leaf) {
				seen[setting] = true
			}
		}
	}

	for _, setting := range config.Settings() {
		if !seen[setting] {
			t.Errorf("setting %q is never shown under any forge kind", setting)
		}
	}
}

// lastSegment returns a setting's final path segment, which is what the
// listing prints.
func lastSegment(setting string) (string, string) {
	index := strings.LastIndex(setting, ".")
	if index < 0 {
		return "", setting
	}

	return setting[:index], setting[index+1:]
}

func TestConfigResetRefusesWithoutATerminal(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	existing := document("/srv/workspaces")
	writeDocument(t, env, existing)

	// A "y" piped from a script is not a person confirming, and this is the
	// most destructive thing the config commands do.
	out, err := runCmd(t, env, "y\n", "config", "reset")
	if err == nil {
		t.Fatal("Execute() error = nil, want a refusal without a terminal")
	}

	if !strings.Contains(out, "--yes") {
		t.Errorf("error does not say how to confirm, got:\n%s", out)
	}

	if got := readDocument(t, env); got != existing {
		t.Errorf("document on disk = %q, want it untouched as %q", got, existing)
	}
}
