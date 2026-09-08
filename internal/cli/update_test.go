package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/update"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// newerRelease is what the stub offers when a test wants one to be found.
func newerRelease() update.Release {
	return update.Release{Tag: "v1.3.0", Prerelease: false, Assets: nil}
}

// runUpdateCmd runs args against a tree wired to updater, returning the output.
func runUpdateCmd(t *testing.T, env stubEnv, updater *stubUpdater, args ...string) (string, error) {
	t.Helper()

	return runCmdWith(t, env, updater, "", args...)
}

func TestUpdateReportsWhenAlreadyCurrent(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, found: false}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "update")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "newest") {
		t.Errorf("output = %q, want it to say cwm is current", out)
	}

	if updater.applies != 0 {
		t.Errorf("Apply called %d times, want none when nothing is newer", updater.applies)
	}
}

func TestUpdateInstallsANewerRelease(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, found: true, release: newerRelease(), target: "/usr/local/bin/cwm"}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "update")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"1.3.0", "/usr/local/bin/cwm"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to mention %q", out, want)
		}
	}

	if updater.applies != 1 {
		t.Errorf("Apply called %d times, want 1", updater.applies)
	}
}

func TestUpdateCheckDoesNotInstall(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, found: true, release: newerRelease()}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "update", "check")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "1.3.0") || !strings.Contains(out, "cwm update") {
		t.Errorf("output = %q, want it to name the release and how to install it", out)
	}

	if updater.applies != 0 {
		t.Errorf("Apply called %d times, want none from a check", updater.applies)
	}
}

func TestUpdateUsesTheConfiguredChannel(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(
		t,
		env,
		`{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"auto","channel":"prerelease"}}`,
	)

	updater := &stubUpdater{updatable: true, found: false}

	if _, err := runUpdateCmd(t, env, updater, "update", "check"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(updater.channels) != 1 || updater.channels[0] != config.ChannelPrerelease {
		t.Errorf("channels = %v, want [prerelease]", updater.channels)
	}
}

func TestUpdatePreOverridesTheChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"on the update command", []string{"update", "--pre"}},
		{"on the check subcommand", []string{"update", "check", "--pre"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The document says stable; --pre has to win for this one run.
			env := newStubEnv(t)
			writeDocument(t, env,
				`{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"auto","channel":"stable"}}`)

			updater := &stubUpdater{updatable: true, found: false}

			if _, err := runUpdateCmd(t, env, updater, tt.args...); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if len(updater.channels) != 1 || updater.channels[0] != config.ChannelPrerelease {
				t.Errorf("channels = %v, want [prerelease]", updater.channels)
			}
		})
	}
}

func TestUpdateRefusesOnABuildWithNoReleaseBehindIt(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: false}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "update")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for a dev build")
	}

	if !strings.Contains(out, "not a published release") {
		t.Errorf("output = %q, want it to explain why", out)
	}

	if updater.checks != 0 {
		t.Errorf("Check called %d times, want none", updater.checks)
	}
}

func TestUpdateReportsACheckFailure(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, checkErr: errors.New("github is down")}

	_, err := runUpdateCmd(t, newStubEnv(t), updater, "update")
	if err == nil {
		t.Fatal("Execute() error = nil, want the check failure reported")
	}

	// An explicitly requested check still records the attempt, so a failure
	// does not make the next command try again immediately.
	if updater.marks != 1 {
		t.Errorf("MarkChecked called %d times, want 1", updater.marks)
	}
}

func TestUpdateReportsAnInstallFailure(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{
		updatable: true,
		found:     true,
		release:   newerRelease(),
		applyErr:  errors.New("permission denied"),
	}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "update")
	if err == nil {
		t.Fatal("Execute() error = nil, want the install failure reported")
	}

	if !strings.Contains(out, "permission denied") {
		t.Errorf("output = %q, want the underlying reason", out)
	}
}

func TestUpdateRejectsArgs(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "update", "unexpected"); err == nil {
		t.Error("Execute() error = nil, want an error for an unexpected argument")
	}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "update", "check", "unexpected"); err == nil {
		t.Error("Execute() error = nil, want an error for an unexpected argument")
	}
}

func TestAutoCheckNagsInCheckMode(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"check","channel":"stable"}}`)

	updater := &stubUpdater{updatable: true, due: true, found: true, release: newerRelease()}

	out, err := runUpdateCmd(t, env, updater, "version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "1.3.0") {
		t.Errorf("output = %q, want the nag", out)
	}

	if updater.applies != 0 {
		t.Errorf("Apply called %d times, want none in check mode", updater.applies)
	}
}

func TestAutoCheckInstallsInAutoMode(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"auto","channel":"stable"}}`)

	updater := &stubUpdater{
		updatable: true,
		due:       true,
		found:     true,
		release:   newerRelease(),
		target:    "/home/u/bin/cwm",
	}

	out, err := runUpdateCmd(t, env, updater, "version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if updater.applies != 1 {
		t.Errorf("Apply called %d times, want 1", updater.applies)
	}

	if !strings.Contains(out, "updated itself to 1.3.0") {
		t.Errorf("output = %q, want it to say what happened", out)
	}
}

func TestAutoCheckStaysQuietWhenNotDue(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, due: false, found: true, release: newerRelease()}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "version"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if updater.checks != 0 {
		t.Errorf("Check called %d times, want none before the interval is up", updater.checks)
	}
}

func TestAutoCheckSkipsABuildWithNoReleaseBehindIt(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: false, due: true}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "version"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if updater.checks != 0 {
		t.Errorf("Check called %d times, want none on a dev build", updater.checks)
	}
}

func TestAutoCheckNeverFailsTheCommand(t *testing.T) {
	t.Parallel()

	// Someone running "cwm version" asked about their cwm, not about GitHub.
	updater := &stubUpdater{updatable: true, due: true, checkErr: errors.New("github is down")}

	out, err := runUpdateCmd(t, newStubEnv(t), updater, "version")
	if err != nil {
		t.Fatalf("Execute() error = %v, want the command to succeed anyway", err)
	}

	if !strings.Contains(out, "version:") {
		t.Errorf("output = %q, want the command's own output", out)
	}

	if strings.Contains(out, "github is down") {
		t.Errorf("output = %q, want a failed background check to stay quiet", out)
	}
}

func TestAutoCheckReportsAFailedAutomaticInstall(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"auto","channel":"stable"}}`)

	// The user asked for automatic updates and is not getting them, which is
	// worth one line even though the command itself is fine.
	updater := &stubUpdater{
		updatable: true,
		due:       true,
		found:     true,
		release:   newerRelease(),
		applyErr:  errors.New("permission denied"),
	}

	out, err := runUpdateCmd(t, env, updater, "version")
	if err != nil {
		t.Fatalf("Execute() error = %v, want the command to succeed anyway", err)
	}

	if !strings.Contains(out, "could not install") {
		t.Errorf("output = %q, want a note about the failed install", out)
	}
}

func TestAutoCheckStaysOutOfTheUpdateCommands(t *testing.T) {
	t.Parallel()

	updater := &stubUpdater{updatable: true, due: true, found: false}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "update", "check"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// One check, from the command itself, not two.
	if updater.checks != 1 {
		t.Errorf("Check called %d times, want 1", updater.checks)
	}
}

func TestAutoCheckRecordsBeforeChecking(t *testing.T) {
	t.Parallel()

	// The timestamp is written even when the check then fails, so a machine
	// with no network does not retry on every single command.
	updater := &stubUpdater{updatable: true, due: true, checkErr: errors.New("github is down")}

	if _, err := runUpdateCmd(t, newStubEnv(t), updater, "version"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if updater.marks != 1 {
		t.Errorf("MarkChecked called %d times, want 1", updater.marks)
	}
}
