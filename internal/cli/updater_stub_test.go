package cli_test

import (
	"context"
	"errors"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
	"github.com/pdylanross/claude-workspace-manager/internal/forge"
	"github.com/pdylanross/claude-workspace-manager/internal/update"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// stubUpdater stands in for the real updater so no test touches the network.
type stubUpdater struct {
	updatable bool
	due       bool
	release   update.Release
	found     bool
	checkErr  error
	applyErr  error
	target    string

	// Recorded so a test can assert what the command did rather than only what
	// it printed.
	checks   int
	applies  int
	marks    int
	channels []config.Channel
}

func (s *stubUpdater) Updatable() bool { return s.updatable }

func (s *stubUpdater) Due() bool { return s.due }

func (s *stubUpdater) MarkChecked() error {
	s.marks++

	return nil
}

func (s *stubUpdater) Check(_ context.Context, channel config.Channel) (update.Release, bool, error) {
	s.checks++
	s.channels = append(s.channels, channel)

	if s.checkErr != nil {
		return update.Release{}, false, s.checkErr
	}

	return s.release, s.found, nil
}

func (s *stubUpdater) Apply(_ context.Context, _ update.Release) (string, error) {
	s.applies++

	if s.applyErr != nil {
		return "", s.applyErr
	}

	return s.target, nil
}

// factory returns a factory handing out this stub.
func (s *stubUpdater) factory() cli.UpdaterFactory {
	return func() (cli.Updater, error) { return s, nil }
}

// stubUpdaterFactory is the do-nothing updater the tests that are not about
// updating are built with: a build with no release behind it never checks.
func stubUpdaterFactory() cli.UpdaterFactory {
	return (&stubUpdater{updatable: false, due: false}).factory()
}

// stubToolFactory hands out a forge client probe that reports nothing
// installed, so no test needs gh or glab present.
func stubToolFactory() cli.ToolFactory {
	return readyToolFactory(false, "")
}

// readyToolFactory hands out a probe that reports a client which is installed
// and logged in as account, or one that is not installed at all.
func readyToolFactory(ready bool, account string) cli.ToolFactory {
	return func(kind config.ForgeKind) (*forge.Tool, error) {
		return forge.New(kind, forge.Options{
			LookPath: func(command string) (string, error) {
				if !ready {
					return "", errNoForgeClient
				}

				return "/usr/bin/" + command, nil
			},
			Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				if !ready {
					return nil, errNoForgeClient
				}

				if len(args) > 0 && args[0] == "auth" {
					return []byte("  Token scopes: 'repo', 'read:org'\n"), nil
				}

				return []byte(account + "\n"), nil
			},
		})
	}
}

// errNoForgeClient is what the fake reports when nothing is installed.
var errNoForgeClient = errors.New("no forge client in tests")
