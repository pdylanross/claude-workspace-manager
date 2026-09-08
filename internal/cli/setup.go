package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/internal/forge"
	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// errNotReady is returned when a forge's client cannot be used yet.
var errNotReady = errors.New("forge client is not ready")

// setupFlags are the answers a run can be given rather than asked for.
//
// Every question below has one, per SKILL.md §6: a command that can only be
// driven by answering questions cannot be scripted or tested.
type setupFlags struct {
	forge         string
	space         string
	host          string
	workspaceRoot string
	assumeYes     bool
}

// newSetupCmd builds the "cwm setup" command.
func newSetupCmd(resolver *paths.Resolver, newTool toolFactory) *cobra.Command {
	var flags setupFlags

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Point cwm at a forge",
		Long: "Choose the forge and space cwm manages workspaces in, and where clones live.\n\n" +
			"cwm does not hold credentials of its own: it reads them from the forge's own\n" +
			"command line client, per run, so that client has to be installed and logged in\n" +
			"first. Setup checks that and says what to run when it is not.\n\n" +
			"Nothing is created or cloned here. Re-running is safe: the current configuration\n" +
			"becomes the defaults.",
		Example: "  cwm setup\n" +
			"  cwm setup --forge github --space pdylanross --yes",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, resolver, newTool, flags)
		},
	}

	cmd.Flags().StringVar(&flags.forge, "forge", "", "forge to use: "+strings.Join(forgeChoices(), " or "))
	cmd.Flags().StringVar(&flags.space, "space", "", "account, organisation or group holding the workspaces")
	cmd.Flags().StringVar(&flags.host, "host", "", "GitLab instance, empty for gitlab.com")
	cmd.Flags().StringVar(&flags.workspaceRoot, "workspace-root", "", "directory the clones live in")
	cmd.Flags().BoolVarP(&flags.assumeYes, "yes", "y", false, "do not ask for confirmation")

	return cmd
}

// runSetup asks for whatever the flags did not supply, then writes it.
func runSetup(cmd *cobra.Command, resolver *paths.Resolver, newTool toolFactory, flags setupFlags) error {
	store, err := newConfigStore(resolver)
	if err != nil {
		return err
	}

	cfg, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load the configuration: %w", err)
	}

	kind, err := chooseForge(cmd, cfg, flags)
	if err != nil {
		return err
	}

	tool, err := newTool(kind)
	if err != nil {
		return fmt.Errorf("check the forge client: %w", err)
	}

	status := tool.Check(cmd.Context())
	if !status.Ready() {
		return fmt.Errorf("%w: %s", errNotReady, status.Advice())
	}

	if statusErr := writeStatus(cmd, status); statusErr != nil {
		return statusErr
	}

	updated, err := gatherSettings(cmd, cfg, kind, status, flags)
	if err != nil {
		return err
	}

	confirmed, err := confirmSetup(cmd, updated, flags)
	if err != nil {
		return err
	}

	if !confirmed {
		return writeOut(cmd, []byte("cancelled, nothing was written\n"))
	}

	if saveErr := store.Save(cmd.Context(), updated); saveErr != nil {
		return fmt.Errorf("save the configuration: %w", saveErr)
	}

	return writeOut(cmd, fmt.Appendf(nil, "written to %s\n", store.Path()))
}

// chooseForge settles which forge, from the flag, the configuration, or a
// question.
func chooseForge(cmd *cobra.Command, cfg config.Config, flags setupFlags) (config.ForgeKind, error) {
	if flags.forge != "" {
		kind := config.ForgeKind(flags.forge)
		if kind != config.ForgeGitHub && kind != config.ForgeGitLab {
			return "", fmt.Errorf(
				"--forge is %q, but must be one of: %s",
				flags.forge,
				strings.Join(forgeChoices(), ", "),
			)
		}

		return kind, nil
	}

	if !isInteractive(cmd) {
		return "", fmt.Errorf("%w: pass --forge", errNotATerminal)
	}

	kind := cfg.Forge.Kind
	if !cfg.Forge.Configured() {
		kind = config.ForgeGitHub
	}

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[config.ForgeKind]().
				Title("Which forge?").
				Options(
					huh.NewOption("GitHub", config.ForgeGitHub),
					huh.NewOption("GitLab", config.ForgeGitLab),
				).
				Value(&kind),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).RunWithContext(cmd.Context())
	if err != nil {
		return "", fmt.Errorf("choose a forge: %w", err)
	}

	return kind, nil
}

// gatherSettings fills in the space, the host and the workspace root.
func gatherSettings(
	cmd *cobra.Command,
	cfg config.Config,
	kind config.ForgeKind,
	status forge.Status,
	flags setupFlags,
) (config.Config, error) {
	space, err := ask(cmd, question{
		title:    "Space",
		help:     "The account, organisation or group holding the workspaces.",
		flag:     "--space",
		value:    firstNonEmpty(flags.space, currentSpace(cfg, kind), status.Account),
		given:    flags.space != "",
		optional: false,
	})
	if err != nil {
		return config.Config{}, err
	}

	host := ""

	if kind == config.ForgeGitLab {
		host, err = ask(cmd, question{
			title:    "Host",
			help:     "The GitLab instance. Leave empty for gitlab.com.",
			flag:     "--host",
			value:    firstNonEmpty(flags.host, cfg.Forge.GitLab.Host, status.Host),
			given:    flags.host != "",
			optional: true,
		})
		if err != nil {
			return config.Config{}, err
		}
	}

	root, err := ask(cmd, question{
		title:    "Workspace root",
		help:     "The directory clones live in.",
		flag:     "--workspace-root",
		value:    firstNonEmpty(flags.workspaceRoot, cfg.WorkspaceRoot),
		given:    flags.workspaceRoot != "",
		optional: false,
	})
	if err != nil {
		return config.Config{}, err
	}

	updated := cfg
	updated.WorkspaceRoot = root
	updated.Forge.Kind = kind

	switch kind {
	case config.ForgeGitHub:
		updated.Forge.GitHub.Space = space
	case config.ForgeGitLab:
		updated.Forge.GitLab.Space = space
		updated.Forge.GitLab.Host = host
	case config.ForgeNone:
	default:
	}

	return updated, nil
}

// question is one thing setup needs an answer to.
type question struct {
	title    string
	help     string
	flag     string
	value    string
	given    bool
	optional bool
}

// ask returns the answer to a question, prompting only when it has to.
//
// A value supplied by a flag is taken as given and never re-asked. Without a
// terminal there is nobody to ask, so a question that still needs answering
// names the flag that would have answered it rather than guessing.
func ask(cmd *cobra.Command, q question) (string, error) {
	if q.given {
		return q.value, nil
	}

	if !isInteractive(cmd) {
		if q.value != "" || q.optional {
			return q.value, nil
		}

		return "", fmt.Errorf("%w: pass %s", errNotATerminal, q.flag)
	}

	answer := q.value

	input := huh.NewInput().Title(q.title).Description(q.help).Value(&answer)
	if !q.optional {
		input = input.Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("required")
			}

			return nil
		})
	}

	err := huh.NewForm(huh.NewGroup(input)).
		WithInput(cmd.InOrStdin()).
		WithOutput(cmd.OutOrStdout()).
		RunWithContext(cmd.Context())
	if err != nil {
		return "", fmt.Errorf("ask for %s: %w", q.title, err)
	}

	return strings.TrimSpace(answer), nil
}

// confirmSetup shows what is about to be written and asks whether to write it.
func confirmSetup(cmd *cobra.Command, cfg config.Config, flags setupFlags) (bool, error) {
	if flags.assumeYes {
		return true, nil
	}

	listing, err := renderSettings(cmd, cfg, "")
	if err != nil {
		return false, err
	}

	if writeErr := writeOut(cmd, []byte("\n"+listing+"\n")); writeErr != nil {
		return false, writeErr
	}

	return confirm(cmd, "Write this configuration?", "--yes")
}

// writeStatus reports what the forge's client said about itself.
func writeStatus(cmd *cobra.Command, status forge.Status) error {
	line := fmt.Sprintf("%s is installed and authenticated", status.Command)
	if status.Account != "" {
		line += " as " + status.Account
	}

	// Missing delete_repo is a note rather than a failure: it is the one scope
	// a default login omits, and everything except deleting a workspace works
	// without it.
	if len(status.Scopes) > 0 && !status.HasScope(forge.ScopeDeleteRepo) {
		line += "\nnote: the token cannot delete repositories; `" + status.Command +
			" auth refresh -s " + forge.ScopeDeleteRepo + "` grants that"
	}

	return writeOut(cmd, []byte(line+"\n"))
}

// currentSpace returns the space already configured for a forge.
func currentSpace(cfg config.Config, kind config.ForgeKind) string {
	switch kind {
	case config.ForgeGitHub:
		return cfg.Forge.GitHub.Space
	case config.ForgeGitLab:
		return cfg.Forge.GitLab.Space
	case config.ForgeNone:
		return ""
	default:
		return ""
	}
}

// forgeChoices lists the forges setup will accept.
func forgeChoices() []string {
	return []string{string(config.ForgeGitHub), string(config.ForgeGitLab)}
}

// firstNonEmpty returns the first value that has one.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
