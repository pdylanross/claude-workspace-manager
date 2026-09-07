package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// newVersionCmd builds the "cwm version" command.
func newVersionCmd(info version.Info) *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the cwm version",
		Long: "Print build information for this cwm binary: version, commit, build date, and\n" +
			"the Go toolchain and platform it was built for.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			text := info.String()
			if short {
				text = info.Short() + "\n"
			}

			if _, err := io.WriteString(cmd.OutOrStdout(), text); err != nil {
				return fmt.Errorf("write version output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "print only the version string")

	return cmd
}
