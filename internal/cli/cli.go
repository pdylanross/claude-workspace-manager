// Package cli builds the cwm command tree.
//
// It exists so that cmd/cwm stays a thin entrypoint whose only job is to hold
// the link-time build variables.
package cli

import (
	"context"
	"fmt"

	"github.com/charmbracelet/fang"

	"github.com/pdylanross/claude-workspace-manager/internal/forge"
	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// Execute builds the cwm command tree and runs it against [os.Args].
//
// Errors are returned rather than printed twice: cobra has already written the
// message to stderr by the time this returns.
func Execute(info version.Info) error {
	resolver := paths.New(paths.NewOSEnvironment())

	newUpdater := func() (Updater, error) { return newOSUpdater(resolver, info) }
	newTool := func(kind config.ForgeKind) (*forge.Tool, error) {
		return forge.New(kind, forge.Options{Run: nil, LookPath: nil})
	}

	// fang styles help, usage, errors and --version, and owns the version flag,
	// which is why newRootCmd leaves cobra's Version field alone.
	err := fang.Execute(
		context.Background(),
		newRootCmd(info, resolver, newUpdater, newTool),
		fang.WithVersion(info.Short()),
		fang.WithCommit(info.Commit),
	)
	if err != nil {
		return fmt.Errorf("execute cwm: %w", err)
	}

	return nil
}
