package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// runSetupCmd runs args against a tree whose forge client reports the given
// state, and returns everything written.
func runSetupCmd(t *testing.T, env stubEnv, ready bool, account string, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := cli.NewRootCmd(
		version.Info{Version: "1.2.0", Commit: "", Date: ""},
		paths.New(env),
		(&stubUpdater{updatable: false, due: false}).factory(),
		readyToolFactory(ready, account),
	)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func TestSetupWritesTheForge(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runSetupCmd(t, env, true, "pdylanross", "setup", "--forge", "github", "--space", "acme", "--yes")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "authenticated as pdylanross") {
		t.Errorf("output missing what the client reported, got:\n%s", out)
	}

	document := readDocument(t, env)
	for _, want := range []string{`"kind": "github"`, `"space": "acme"`} {
		if !strings.Contains(document, want) {
			t.Errorf("document missing %s, got:\n%s", want, document)
		}
	}
}

func TestSetupDefaultsTheSpaceToTheAccount(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	// Asking someone to type what the client beside it already knows is a bad
	// first impression, so an unsupplied space falls back to the account.
	if _, err := runSetupCmd(t, env, true, "pdylanross", "setup", "--forge", "github", "--yes"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readDocument(t, env); !strings.Contains(got, `"space": "pdylanross"`) {
		t.Errorf("document missing the account as the space, got:\n%s", got)
	}
}

func TestSetupRefusesWhenTheClientIsNotReady(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	out, err := runSetupCmd(t, env, false, "", "setup", "--forge", "github", "--yes")
	if err == nil {
		t.Fatal("Execute() error = nil, want a refusal when gh is not installed")
	}

	// The command exists to say what to run.
	if !strings.Contains(out, "gh auth login") {
		t.Errorf("error does not name the command to run, got:\n%s", out)
	}

	// Loading the configuration materialises the defaults, as every command
	// does, so a document exists. What must not have happened is the forge
	// being recorded against a client that cannot be used.
	if got := readDocument(t, env); !strings.Contains(got, `"kind": "none"`) {
		t.Errorf("the forge was configured despite the client not being ready, got:\n%s", got)
	}
}

func TestSetupRejectsAnUnknownForge(t *testing.T) {
	t.Parallel()

	out, err := runSetupCmd(t, newStubEnv(t), true, "someone", "setup", "--forge", "bitbucket", "--yes")
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for an unknown forge")
	}

	if !strings.Contains(out, "github") || !strings.Contains(out, "gitlab") {
		t.Errorf("error does not list the forges, got:\n%s", out)
	}
}

func TestSetupNeedsTheForgeWithoutATerminal(t *testing.T) {
	t.Parallel()

	out, err := runSetupCmd(t, newStubEnv(t), true, "someone", "setup", "--yes")
	if err == nil {
		t.Fatal("Execute() error = nil, want a refusal with nothing to ask")
	}

	if !strings.Contains(out, "--forge") {
		t.Errorf("error does not name the flag to pass, got:\n%s", out)
	}
}

func TestSetupNotesAMissingDeleteScope(t *testing.T) {
	t.Parallel()

	// A default gh login cannot delete repositories. That is worth saying and
	// not worth failing over, since everything else works without it.
	out, err := runSetupCmd(t, newStubEnv(t), true, "pdylanross", "setup", "--forge", "github", "--yes")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "delete_repo") {
		t.Errorf("output missing the note about deletion, got:\n%s", out)
	}
}

func TestSetupKeepsTheWorkspaceRoot(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)
	writeDocument(t, env, `{"schemaVersion":1,"workspaceRoot":"/srv/ws","update":{"mode":"auto","channel":"stable"}}`)

	if _, err := runSetupCmd(t, env, true, "pdylanross", "setup", "--forge", "github", "--yes"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Re-running setup must not quietly move where the clones live.
	if got := readDocument(t, env); !strings.Contains(got, `"workspaceRoot": "/srv/ws"`) {
		t.Errorf("document lost the configured workspace root, got:\n%s", got)
	}
}

func TestSetupGitLabRecordsTheHost(t *testing.T) {
	t.Parallel()

	env := newStubEnv(t)

	_, err := runSetupCmd(t, env, true, "dylan",
		"setup", "--forge", "gitlab", "--space", "work/team", "--host", "gitlab.example.com", "--yes")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	document := readDocument(t, env)
	for _, want := range []string{`"kind": "gitlab"`, `"host": "gitlab.example.com"`, `"space": "work/team"`} {
		if !strings.Contains(document, want) {
			t.Errorf("document missing %s, got:\n%s", want, document)
		}
	}
}
