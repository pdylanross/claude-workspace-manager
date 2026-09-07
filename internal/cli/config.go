package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// newConfigCmd builds the "cwm config" command group.
//
// It has no run of its own; cobra prints its help and lists the subcommands.
func newConfigCmd(resolver *paths.Resolver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect cwm's configuration",
		Long:  "Inspect the configuration cwm keeps for itself: where it lives and what it says.",
	}

	cmd.AddCommand(newConfigPathsCmd(resolver), newConfigShowCmd(resolver))

	return cmd
}

// newConfigShowCmd builds the "cwm config show" command.
func newConfigShowCmd(resolver *paths.Resolver) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the cwm configuration",
		Long: "Print the cwm configuration document as JSON.\n\n" +
			"If no document exists yet, one is written with cwm's defaults and printed, so\n" +
			"the output always matches what is on disk.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolver.ConfigRoot()
			if err != nil {
				return fmt.Errorf("resolve the config root: %w", err)
			}

			cfg, err := config.NewStore(root).Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load the configuration: %w", err)
			}

			data, err := cfg.Encode()
			if err != nil {
				return fmt.Errorf("render the configuration: %w", err)
			}

			if _, writeErr := cmd.OutOrStdout().Write(data); writeErr != nil {
				return fmt.Errorf("write the configuration: %w", writeErr)
			}

			return nil
		},
	}
}

// newConfigPathsCmd builds the "cwm config paths" command.
func newConfigPathsCmd(resolver *paths.Resolver) *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Print the directories cwm reads and writes",
		Long: "Print the config root and the cache root.\n\n" +
			"The config root defaults to cwm's directory under the OS config directory and\n" +
			"the cache root to cwm's directory under the OS cache directory. Set\n" +
			paths.ConfigRootEnv + " or " + paths.CacheRootEnv + " to override either.\n\n" +
			"Everything under the cache root is safe to delete.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configRoot, err := resolver.ConfigRoot()
			if err != nil {
				return fmt.Errorf("resolve the config root: %w", err)
			}

			cacheRoot, err := resolver.CacheRoot()
			if err != nil {
				return fmt.Errorf("resolve the cache root: %w", err)
			}

			report := renderPaths(configRoot, cacheRoot)
			if _, writeErr := io.WriteString(cmd.OutOrStdout(), report); writeErr != nil {
				return fmt.Errorf("write the paths output: %w", writeErr)
			}

			return nil
		},
	}
}

// renderPaths lays the reported locations out as aligned "key: value" lines,
// matching the shape of "cwm version".
func renderPaths(configRoot, cacheRoot string) string {
	var b strings.Builder

	for _, row := range []struct{ key, value string }{
		{"config root", configRoot},
		{"cache root", cacheRoot},
	} {
		fmt.Fprintf(&b, "%-13s %s\n", row.key+":", row.value)
	}

	return b.String()
}
