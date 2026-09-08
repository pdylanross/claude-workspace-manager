package cli

import (
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// newRootCmd assembles the cwm root command and all of its subcommands.
func newRootCmd(info version.Info, resolver *paths.Resolver, newUpdater updaterFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cwm",
		Short: "Manage Claude workspaces",
		Long: "cwm manages Claude workspaces: markdown-first repositories used for working with\n" +
			"Claude on things that aren't code.\n\n" +
			"It keeps those workspaces in order on disk and on their remotes, and launches\n" +
			"Claude Code sessions inside them.",
		// Version is deliberately unset: fang owns --version, and setting both
		// gives one flag two implementations. See references/charm.md.
		//
		// Usage on a runtime error is noise; the error message is the useful part.
		SilenceUsage: true,
		// Every command gets the once-a-day update check. It returns nothing,
		// because a check that failed must never fail the command the user
		// actually asked for.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			autoUpdate(cmd, info, resolver, newUpdater)
		},
	}

	cmd.AddCommand(newVersionCmd(info), newConfigCmd(resolver), newUpdateCmd(info, resolver, newUpdater))

	return cmd
}
