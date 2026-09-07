package cli

import (
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// NewRootCmd exposes the unexported root command builder to the cli_test package.
func NewRootCmd(info version.Info) *cobra.Command {
	return newRootCmd(info)
}
