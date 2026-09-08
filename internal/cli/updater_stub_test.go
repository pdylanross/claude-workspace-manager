package cli_test

import (
	"context"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
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
