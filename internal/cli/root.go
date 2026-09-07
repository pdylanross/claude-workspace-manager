package cli

import (
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// newRootCmd assembles the cwm root command and all of its subcommands.
func newRootCmd(info version.Info) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cwm",
		Short: "Manage Claude workspaces",
		Long: "cwm manages Claude workspaces: markdown-first repositories used for working with\n" +
			"Claude on things that aren't code.\n\n" +
			"It keeps those workspaces in order on disk and on their remotes, and launches\n" +
			"Claude Code sessions inside them.",
		Version: info.Short(),
		// Usage on a runtime error is noise; the error message is the useful part.
		SilenceUsage: true,
	}

	cmd.AddCommand(newVersionCmd(info))

	return cmd
}
