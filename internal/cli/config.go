package cli

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// assignment separates a setting from its value in a "cwm config set" argument.
const assignment = "="

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
		newConfigSetCmd(resolver),
		newConfigResetCmd(resolver),
	)

	return cmd
}

// newConfigShowCmd builds the "cwm config show" command.
func newConfigShowCmd(resolver *paths.Resolver) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show [SETTING]",
		Short: "Print the cwm configuration",
		Long: "Print the cwm configuration.\n\n" +
			"Settings that do nothing in the current configuration are left out: a cwm set\n" +
			"up against GitHub is not shown its GitLab settings. Use --json for the document\n" +
			"exactly as it is on disk, which is what a script should read.\n\n" +
			"Given a setting, print only that setting's value, unquoted so that a shell can\n" +
			"use it directly.\n\n" +
			"If no document exists yet, one is written with cwm's defaults first, so the\n" +
			"output always matches what is on disk.",
		Example: "  cwm config show\n" +
			"  cwm config show --json\n" +
			"  cwm config show workspaceRoot",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeSettings,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newConfigStore(resolver)
			if err != nil {
				return err
			}

			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load the configuration: %w", err)
			}

			if asJSON {
				return showJSON(cmd, cfg, args)
			}

			return showSettings(cmd, cfg, args)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print the document as it is on disk")

	return cmd
}

// showSettings prints the configuration for a person to read.
func showSettings(cmd *cobra.Command, cfg config.Config, args []string) error {
	prefix := ""

	if len(args) > 0 {
		prefix = args[0]

		// Addressing it first is what keeps the error worth reading: an
		// unknown setting is reported by the layer that knows what the known
		// ones are.
		value, err := cfg.Get(prefix)
		if err != nil {
			return fmt.Errorf("show %s: %w", prefix, err)
		}

		// A single setting stays bare and unstyled however it is asked for:
		// "$(cwm config show workspaceRoot)" is the point of it.
		if !isGroup(value) {
			data, renderErr := config.Render(value)
			if renderErr != nil {
				return fmt.Errorf("render %s: %w", prefix, renderErr)
			}

			return writeOut(cmd, data)
		}
	}

	listing, err := renderSettings(cmd, cfg, prefix)
	if err != nil {
		return err
	}

	return writeOut(cmd, []byte(listing))
}

// showJSON prints the document, or one value from it, as JSON.
func showJSON(cmd *cobra.Command, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return writeConfig(cmd, cfg)
	}

	value, err := cfg.Get(args[0])
	if err != nil {
		return fmt.Errorf("show %s: %w", args[0], err)
	}

	data, err := config.Render(value)
	if err != nil {
		return fmt.Errorf("render %s: %w", args[0], err)
	}

	return writeOut(cmd, data)
}

// isGroup reports whether an addressed value is a group of settings rather than
// one setting.
func isGroup(value any) bool {
	return reflect.ValueOf(value).Kind() == reflect.Struct
}

// newConfigSetCmd builds the "cwm config set" command.
func newConfigSetCmd(resolver *paths.Resolver) *cobra.Command {
	return &cobra.Command{
		Use:   "set SETTING=VALUE [SETTING=VALUE ...]",
		Short: "Change cwm configuration settings",
		Long: "Change one or more settings and write the document back.\n\n" +
			"Settings are named by path, with a dot for each level: workspaceRoot for a\n" +
			"top-level setting, github.repoPrefix for one inside a group. Values are parsed\n" +
			"according to the setting's type, so a setting that holds true or false rejects\n" +
			"anything else.\n\n" +
			"Every assignment is applied before anything is written, so a run that fails\n" +
			"part way through leaves the document untouched. Assigning an empty value puts\n" +
			"a setting back to its default.",
		Example: "  cwm config set workspaceRoot=/srv/workspaces\n" +
			"  cwm config set github.enabled=true github.repoPrefix=cwm-",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeAssignments,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newConfigStore(resolver)
			if err != nil {
				return err
			}

			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load the configuration: %w", err)
			}

			updated, err := applyAssignments(cfg, args)
			if err != nil {
				return err
			}

			// Normalise before validating and printing, so the output is what
			// Save writes and what the next read reports: a cleared setting
			// shows its default, and "~/ws" shows the expanded path.
			updated = store.Normalize(updated)

			if validateErr := updated.Validate(); validateErr != nil {
				return fmt.Errorf("apply the changes: %w", validateErr)
			}

			if saveErr := store.Save(cmd.Context(), updated); saveErr != nil {
				return fmt.Errorf("save the configuration: %w", saveErr)
			}

			return writeConfig(cmd, updated)
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
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := newConfigStore(resolver)
			if err != nil {
				return err
			}

			if !assumeYes {
				confirmed, confirmErr := confirm(cmd, "Discard "+store.Path()+" and take the defaults?", "--yes")
				if confirmErr != nil {
					return confirmErr
				}

				if !confirmed {
					return writeOut(cmd, []byte("cancelled, nothing was written\n"))
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
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configRoot, err := resolver.ConfigRoot()
			if err != nil {
				return fmt.Errorf("resolve the config root: %w", err)
			}

			cacheRoot, err := resolver.CacheRoot()
			if err != nil {
				return fmt.Errorf("resolve the cache root: %w", err)
			}

			return writeOut(cmd, []byte(renderPaths(configRoot, cacheRoot)))
		},
	}
}

// applyAssignments parses "setting=value" arguments and applies them all to cfg.
//
// Nothing is applied to the caller's copy until every assignment has parsed, so
// a bad argument in the middle of a run changes nothing.
func applyAssignments(cfg config.Config, args []string) (config.Config, error) {
	updated := cfg

	for _, arg := range args {
		setting, value, ok := strings.Cut(arg, assignment)
		if !ok {
			return config.Config{}, fmt.Errorf(
				"%q is not a setting assignment; write it as SETTING%sVALUE", arg, assignment,
			)
		}

		setting = strings.TrimSpace(setting)
		if setting == "" {
			return config.Config{}, fmt.Errorf("%q does not name a setting", arg)
		}

		next, err := updated.Set(setting, value)
		if err != nil {
			return config.Config{}, fmt.Errorf("apply %q: %w", arg, err)
		}

		updated = next
	}

	return updated, nil
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

	return config.NewStore(root, home), nil
}

// completeSettings completes a setting name.
func completeSettings(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return config.Settings(), cobra.ShellCompDirectiveNoFileComp
}

// completeAssignments completes the setting half of a "setting=value" argument,
// leaving the cursor against the separator so a value can be typed.
func completeAssignments(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	settings := config.Settings()

	assignments := make([]cobra.Completion, 0, len(settings))
	for _, setting := range settings {
		assignments = append(assignments, setting+assignment)
	}

	return assignments, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

// writeConfig prints cfg as JSON on the command's output.
func writeConfig(cmd *cobra.Command, cfg config.Config) error {
	data, err := cfg.Encode()
	if err != nil {
		return fmt.Errorf("render the configuration: %w", err)
	}

	return writeOut(cmd, data)
}

// writeOut writes data to the command's output.
func writeOut(cmd *cobra.Command, data []byte) error {
	if _, err := cmd.OutOrStdout().Write(data); err != nil {
		return fmt.Errorf("write the command output: %w", err)
	}

	return nil
}

// errNotATerminal is returned when something that has to be confirmed is run
// without anybody there to confirm it.
var errNotATerminal = errors.New("not a terminal")

// confirm puts question to the user and waits for an answer.
//
// It requires a terminal. Reading a "y" from a pipe would mean a destructive
// command could be confirmed by a script that never meant to, so a run with no
// terminal is refused and told which flag says yes in advance. That flag is the
// non-interactive interface; the prompt is for people.
func confirm(cmd *cobra.Command, question, flag string) (bool, error) {
	if !isInteractive(cmd) {
		return false, fmt.Errorf("%w: pass %s to confirm", errNotATerminal, flag)
	}

	confirmed := false

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&confirmed),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout())

	if err := form.RunWithContext(cmd.Context()); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}

		return false, fmt.Errorf("ask for confirmation: %w", err)
	}

	return confirmed, nil
}

// isInteractive reports whether there is a person on the other end of both
// streams a prompt needs.
func isInteractive(cmd *cobra.Command) bool {
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(in.Fd()) {
		return false
	}

	out, ok := cmd.OutOrStdout().(*os.File)

	return ok && term.IsTerminal(out.Fd())
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
