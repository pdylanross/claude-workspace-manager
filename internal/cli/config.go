package cli

import (
	"bufio"
	"errors"
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

	cmd.AddCommand(
		newConfigPathsCmd(resolver),
		newConfigShowCmd(resolver),
		newConfigResetCmd(resolver),
	)

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
			store, err := newConfigStore(resolver)
			if err != nil {
				return err
			}

			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load the configuration: %w", err)
			}

			return writeConfig(cmd, cfg)
		},
	}
}

// newConfigResetCmd builds the "cwm config reset" command.
func newConfigResetCmd(resolver *paths.Resolver) *cobra.Command {
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Discard the cwm configuration and take the defaults",
		Long: "Overwrite the cwm configuration document with cwm's defaults.\n\n" +
			"Every setting goes, not only the ones that differ from a default, and the\n" +
			"document is rewritten rather than deleted. cwm asks for confirmation first\n" +
			"unless --yes is given; a run with nothing on its input declines.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := newConfigStore(resolver)
			if err != nil {
				return err
			}

			if !assumeYes {
				confirmed, confirmErr := confirm(cmd, "Discard "+store.Path()+" and take the defaults?")
				if confirmErr != nil {
					return confirmErr
				}

				if !confirmed {
					_, writeErr := io.WriteString(cmd.OutOrStdout(), "cancelled, nothing was written\n")
					if writeErr != nil {
						return fmt.Errorf("write the reset output: %w", writeErr)
					}

					return nil
				}
			}

			cfg, err := store.Reset(cmd.Context())
			if err != nil {
				return fmt.Errorf("reset the configuration: %w", err)
			}

			return writeConfig(cmd, cfg)
		},
	}

	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "do not ask for confirmation")

	return cmd
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

// newConfigStore resolves the config root and the defaults that depend on the
// home directory, and returns the store the config commands work through.
func newConfigStore(resolver *paths.Resolver) (*config.Store, error) {
	root, err := resolver.ConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve the config root: %w", err)
	}

	home, err := resolver.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve the home directory: %w", err)
	}

	return config.NewStore(root, config.Default(home)), nil
}

// writeConfig prints cfg as JSON on the command's output.
func writeConfig(cmd *cobra.Command, cfg config.Config) error {
	data, err := cfg.Encode()
	if err != nil {
		return fmt.Errorf("render the configuration: %w", err)
	}

	if _, writeErr := cmd.OutOrStdout().Write(data); writeErr != nil {
		return fmt.Errorf("write the configuration: %w", writeErr)
	}

	return nil
}

// confirm puts question to the user and reads the answer from the command's
// input.
//
// Anything but "y" or "yes" is a no, including an empty answer and an input
// that is already at EOF, so a non-interactive run declines rather than
// destroying something unattended.
func confirm(cmd *cobra.Command, question string) (bool, error) {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", question); err != nil {
		return false, fmt.Errorf("write the confirmation prompt: %w", err)
	}

	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read the confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
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
