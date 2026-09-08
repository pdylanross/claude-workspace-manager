package forge_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/forge"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// ghStatus is what "gh auth status" prints for a logged-in user.
const ghStatus = `github.com
  ✓ Logged in to github.com account pdylanross (keyring)
  - Active account: true
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'repo'
`

// probe builds a Tool whose client is present and answers with the given
// status report and account.
func probe(t *testing.T, kind config.ForgeKind, installed bool, status, account string) *forge.Tool {
	t.Helper()

	missing := errors.New("not installed")

	tool, err := forge.New(kind, forge.Options{
		LookPath: func(command string) (string, error) {
			if !installed {
				return "", missing
			}

			return "/usr/bin/" + command, nil
		},
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if !installed || status == "" {
				return nil, missing
			}

			if len(args) > 0 && args[0] == "auth" {
				return []byte(status), nil
			}

			return []byte(account + "\n"), nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return tool
}

func TestNewRejectsAForgeWithNoClient(t *testing.T) {
	t.Parallel()

	_, err := forge.New(config.ForgeNone, forge.Options{Run: nil, LookPath: nil})
	if !errors.Is(err, forge.ErrUnknownForge) {
		t.Errorf("New(none) error = %v, want ErrUnknownForge", err)
	}
}

func TestCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind config.ForgeKind
		want string
	}{
		{config.ForgeGitHub, "gh"},
		{config.ForgeGitLab, "glab"},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			tool, err := forge.New(tt.kind, forge.Options{Run: nil, LookPath: nil})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			if got := tool.Command(); got != tt.want {
				t.Errorf("Command() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckReadsTheClient(t *testing.T) {
	t.Parallel()

	status := probe(t, config.ForgeGitHub, true, ghStatus, "pdylanross").Check(t.Context())

	if !status.Ready() {
		t.Fatalf("Ready() = false for an installed, authenticated client: %+v", status)
	}

	if status.Account != "pdylanross" {
		t.Errorf("Account = %q, want pdylanross", status.Account)
	}

	if !status.HasScope(forge.ScopeRepo) {
		t.Errorf("Scopes = %v, want them to include %q", status.Scopes, forge.ScopeRepo)
	}

	// The scope a default login omits, which setup reports rather than requires.
	if status.HasScope(forge.ScopeDeleteRepo) {
		t.Errorf("Scopes = %v, want %q absent", status.Scopes, forge.ScopeDeleteRepo)
	}

	if advice := status.Advice(); advice != "" {
		t.Errorf("Advice() = %q, want none when ready", advice)
	}
}

func TestCheckWhenNothingIsInstalled(t *testing.T) {
	t.Parallel()

	status := probe(t, config.ForgeGitHub, false, "", "").Check(t.Context())

	if status.Ready() || status.Installed {
		t.Errorf("Check() = %+v, want it to report nothing installed", status)
	}

	// The whole point of setup is telling somebody what to run.
	if advice := status.Advice(); advice == "" || !strings.Contains(advice, "gh auth login") {
		t.Errorf("Advice() = %q, want it to name the command to run", advice)
	}
}

func TestCheckWhenInstalledButLoggedOut(t *testing.T) {
	t.Parallel()

	// Installed, but "auth status" fails, which is how both clients report
	// having nobody logged in.
	status := probe(t, config.ForgeGitHub, true, "", "").Check(t.Context())

	if !status.Installed {
		t.Error("Installed = false, want true")
	}

	if status.Authenticated || status.Ready() {
		t.Errorf("Check() = %+v, want it to report nobody logged in", status)
	}

	if advice := status.Advice(); !strings.Contains(advice, "auth login") {
		t.Errorf("Advice() = %q, want it to say how to log in", advice)
	}
}

func TestScopesAreBestEffort(t *testing.T) {
	t.Parallel()

	// A client that does not say what the scopes are must not make cwm think
	// the token has none: scope reporting is advice, not a gate.
	status := probe(t, config.ForgeGitHub, true, "logged in, no scope line here\n", "someone").Check(t.Context())

	if !status.Ready() {
		t.Fatalf("Ready() = false, want an authenticated client: %+v", status)
	}

	if len(status.Scopes) != 0 {
		t.Errorf("Scopes = %v, want none parsed", status.Scopes)
	}

	if status.HasScope(forge.ScopeRepo) {
		t.Error("HasScope() = true with no scopes parsed, want false")
	}
}

func TestParsesScopesInOrder(t *testing.T) {
	t.Parallel()

	status := probe(t, config.ForgeGitHub, true, ghStatus, "pdylanross").Check(t.Context())

	if want := []string{"gist", "read:org", "repo"}; !slices.Equal(status.Scopes, want) {
		t.Errorf("Scopes = %v, want %v", status.Scopes, want)
	}
}

func TestGitLabReportsASelfHostedInstance(t *testing.T) {
	t.Parallel()

	// glab is not installed here, so this is the one forge whose probing is
	// only exercised against a fake.
	tool, err := forge.New(config.ForgeGitLab, forge.Options{
		LookPath: func(command string) (string, error) { return "/usr/bin/" + command, nil },
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			switch {
			case len(args) > 0 && args[0] == "auth":
				return []byte("gitlab.example.com:\n  ✓ Logged in\n"), nil
			case len(args) > 1 && args[0] == "config":
				return []byte("gitlab.example.com\n"), nil
			default:
				return []byte("dylan\n"), nil
			}
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	status := tool.Check(t.Context())
	if status.Host != "gitlab.example.com" {
		t.Errorf("Host = %q, want gitlab.example.com", status.Host)
	}
}

func TestGitLabRecordsThePublicInstanceAsNoHost(t *testing.T) {
	t.Parallel()

	tool, err := forge.New(config.ForgeGitLab, forge.Options{
		LookPath: func(command string) (string, error) { return "/usr/bin/" + command, nil },
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 1 && args[0] == "config" {
				return []byte("gitlab.com\n"), nil
			}

			return []byte("gitlab.com:\n  ✓ Logged in\n"), nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// gitlab.com is the default, and cwm records a default as absence rather
	// than as a value.
	if status := tool.Check(t.Context()); status.Host != "" {
		t.Errorf("Host = %q, want empty for the public instance", status.Host)
	}
}
