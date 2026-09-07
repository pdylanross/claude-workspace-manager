// Package cli builds the cwm command tree.
//
// It exists so that cmd/cwm stays a thin entrypoint whose only job is to hold
// the link-time build variables.
package cli

import (
	"fmt"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// Execute builds the cwm command tree and runs it against [os.Args].
//
// Errors are returned rather than printed twice: cobra has already written the
// message to stderr by the time this returns.
func Execute(info version.Info) error {
	resolver := paths.New(paths.NewOSEnvironment())

	if err := newRootCmd(info, resolver).Execute(); err != nil {
		return fmt.Errorf("execute cwm: %w", err)
	}

	return nil
}
