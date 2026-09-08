// Package forge talks to the hosting platforms cwm keeps workspaces on.
//
// Credentials are not cwm's: each forge has its own command line client, that
// client has already solved login, refresh, revocation and keyring storage, and
// cwm borrows a token from it per run rather than storing one. This package is
// the part that asks the client whether it is ready.
package forge

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// Scopes a token needs, beyond being authenticated at all.
const (
	// ScopeRepo is what creating a repository and setting its topics takes. A
	// default "gh auth login" grants it.
	ScopeRepo = "repo"
	// ScopeDeleteRepo is what deleting one takes. A default "gh auth login"
	// does not grant it, which is why it is reported rather than required.
	ScopeDeleteRepo = "delete_repo"
)

// ErrUnknownForge is returned for a forge cwm has no client for.
var ErrUnknownForge = errors.New("unknown forge")

// Runner runs a command and returns its standard output.
//
// It is a parameter so that the probing below can be tested without the real
// clients installed, which is the only way this package is testable at all.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Status is what a forge's client reports about itself.
//
// Not being installed and not being logged in are states rather than errors:
// setup exists to report them, and an error would throw away the detail that
// makes the report worth reading.
type Status struct {
	// Command is the client cwm looked for, "gh" or "glab".
	Command string
	// Installed reports whether it is on PATH.
	Installed bool
	// Authenticated reports whether it is logged in.
	Authenticated bool
	// Account is who it is logged in as, and the obvious default for a space.
	Account string
	// Host is the instance it is logged in to. Empty for the public one.
	Host string
	// Scopes the token carries, when the client will say. Empty means it would
	// not, not that the token has none.
	Scopes []string
}

// Options is what a [Tool] is built from. Both fields may be nil, which means
// the real thing; they exist so a test can probe a client that is not
// installed, which is the only way this is testable on a machine that happens
// to have gh but not glab.
type Options struct {
	// Run runs a command. Nil means [ExecRunner].
	Run Runner
	// LookPath finds a command on PATH. Nil means [exec.LookPath].
	LookPath func(command string) (string, error)
}

// Tool probes one forge's command line client.
type Tool struct {
	kind     config.ForgeKind
	run      Runner
	lookPath func(command string) (string, error)
}

// New returns the Tool for a forge.
func New(kind config.ForgeKind, options Options) (*Tool, error) {
	if command(kind) == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnknownForge, kind)
	}

	tool := &Tool{kind: kind, run: options.Run, lookPath: options.LookPath}

	if tool.run == nil {
		tool.run = ExecRunner
	}

	if tool.lookPath == nil {
		tool.lookPath = exec.LookPath
	}

	return tool, nil
}

// ExecRunner runs a command for real.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return output, fmt.Errorf("run %s: %w", name, err)
	}

	return output, nil
}

// Command returns the client this Tool probes.
func (t *Tool) Command() string {
	return command(t.kind)
}

// Check asks the client what state it is in.
func (t *Tool) Check(ctx context.Context) Status {
	status := Status{
		Command:       t.Command(),
		Installed:     false,
		Authenticated: false,
		Account:       "",
		Host:          "",
		Scopes:        nil,
	}

	if _, err := t.lookPath(status.Command); err != nil {
		return status
	}

	status.Installed = true

	// The status subcommand is the authentication check on both clients: it
	// exits non-zero when nobody is logged in.
	raw, err := t.run(ctx, status.Command, "auth", "status")
	if err != nil {
		return status
	}

	status.Authenticated = true
	status.Scopes = parseScopes(string(raw))
	status.Host = t.host(ctx, string(raw))
	status.Account = t.account(ctx)

	return status
}

// Ready reports whether cwm can use this client.
func (s Status) Ready() bool {
	return s.Installed && s.Authenticated
}

// HasScope reports whether the token is known to carry a scope.
//
// False when the client would not say what the scopes are, so a caller should
// treat this as "not known to have it" rather than "known not to".
func (s Status) HasScope(name string) bool {
	return slicesContains(s.Scopes, name)
}

// Advice returns the command that would move this client forward, and empty
// when it is already ready.
func (s Status) Advice() string {
	switch {
	case !s.Installed:
		return "install " + s.Command + " and log in with `" + s.Command + " auth login`"
	case !s.Authenticated:
		return "log in with `" + s.Command + " auth login`"
	default:
		return ""
	}
}

// account asks the client who it is logged in as.
func (t *Tool) account(ctx context.Context) string {
	var args []string

	switch t.kind {
	case config.ForgeGitHub:
		args = []string{"api", "user", "--jq", ".login"}
	case config.ForgeGitLab:
		args = []string{"api", "user", "--jq", ".username"}
	case config.ForgeNone:
		return ""
	default:
		return ""
	}

	out, err := t.run(ctx, t.Command(), args...)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// host returns the instance the client is logged in to, empty for the public
// one. Only GitLab can be self-hosted in a way cwm cares about.
func (t *Tool) host(ctx context.Context, statusOutput string) string {
	if t.kind != config.ForgeGitLab {
		return ""
	}

	if out, err := t.run(ctx, t.Command(), "config", "get", "host"); err == nil {
		if host := strings.TrimSpace(string(out)); host != "" && host != defaultGitLabHost {
			return host
		}

		return ""
	}

	return parseGitLabHost(statusOutput)
}

// command is the client for a forge, empty when there is none.
func command(kind config.ForgeKind) string {
	switch kind {
	case config.ForgeGitHub:
		return "gh"
	case config.ForgeGitLab:
		return "glab"
	case config.ForgeNone:
		return ""
	default:
		return ""
	}
}
