package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/internal/cache"
	"github.com/pdylanross/claude-workspace-manager/internal/paths"
	"github.com/pdylanross/claude-workspace-manager/internal/update"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// errNotUpdatable is returned when the running build has no release behind it.
var errNotUpdatable = errors.New("cwm cannot update itself")

// updateCmdName is the command the automatic check stays out of, since it is
// the manual version of the same thing.
const updateCmdName = "update"

// downloadTimeout bounds a whole request, including an archive download, so a
// stalled connection cannot hang cwm forever.
const downloadTimeout = 2 * time.Minute

// autoCheckTimeout bounds the check that happens on an unrelated command. It is
// short on purpose: someone running "cwm config show" is not waiting on GitHub.
const autoCheckTimeout = 5 * time.Second

// Updater is what the update commands need. It is declared here, at the
// consumer, so that the commands can be tested without a network.
type Updater interface {
	// Updatable reports whether this build can be replaced by a release.
	Updatable() bool
	// Due reports whether it is time to look for a newer release.
	Due() bool
	// MarkChecked records that a check happened.
	MarkChecked() error
	// Check returns a newer release, if there is one.
	Check(ctx context.Context, channel config.Channel) (update.Release, bool, error)
	// Apply installs a release and returns where it went.
	Apply(ctx context.Context, release update.Release) (string, error)
}

// updaterFactory builds the Updater a command works through.
//
// It is a function rather than a value so that the cache root is resolved when
// a command actually needs it, and so that a failure to resolve it is reported
// by the command rather than at startup.
type updaterFactory func() (Updater, error)

// newUpdateCmd builds the "cwm update" command group.
//
// The group is runnable: "cwm update" on its own is the whole check-and-install
// loop, which is what someone typing it means.
func newUpdateCmd(info version.Info, resolver *paths.Resolver, newUpdater updaterFactory) *cobra.Command {
	var prerelease bool

	cmd := &cobra.Command{
		Use:   updateCmdName,
		Short: "Update cwm to the newest release",
		Long: "Look for a newer cwm and install it.\n\n" +
			"Which releases count is decided by update.channel, and --pre overrides that for\n" +
			"one run. cwm also checks on its own once a day; update.mode decides whether it\n" +
			"installs what it finds or only says so.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd, info, resolver, newUpdater, prerelease, true)
		},
	}

	cmd.PersistentFlags().BoolVar(&prerelease, "pre", false, "consider prereleases, whatever update.channel says")

	cmd.AddCommand(newUpdateCheckCmd(info, resolver, newUpdater))

	return cmd
}

// newUpdateCheckCmd builds the "cwm update check" command.
func newUpdateCheckCmd(info version.Info, resolver *paths.Resolver, newUpdater updaterFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Report whether a newer cwm exists",
		Long: "Look for a newer cwm and say what was found, without installing anything.\n\n" +
			"This is what update.mode=check does on its own once a day.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			prerelease, err := cmd.Flags().GetBool("pre")
			if err != nil {
				return fmt.Errorf("read the --pre flag: %w", err)
			}

			return runUpdate(cmd, info, resolver, newUpdater, prerelease, false)
		},
	}
}

// runUpdate is the body of both update commands: the same check, differing only
// in whether a newer release is installed or merely reported.
func runUpdate(
	cmd *cobra.Command,
	info version.Info,
	resolver *paths.Resolver,
	newUpdater updaterFactory,
	prerelease, install bool,
) error {
	updater, err := newUpdater()
	if err != nil {
		return err
	}

	if !updater.Updatable() {
		return fmt.Errorf(
			"%w: this cwm reports version %q, which is not a published release", errNotUpdatable, info.Short(),
		)
	}

	channel, err := updateChannel(cmd, resolver, prerelease)
	if err != nil {
		return err
	}

	release, found, err := updater.Check(cmd.Context(), channel)

	// Recorded whatever the outcome: a failing check that retried on every
	// command would be worse than a check that waits until tomorrow.
	if markErr := updater.MarkChecked(); markErr != nil && err == nil {
		err = markErr
	}

	if err != nil {
		return fmt.Errorf("check for a newer cwm: %w", err)
	}

	if !found {
		return writeOut(cmd, fmt.Appendf(nil, "cwm %s is the newest on the %s channel\n", info.Short(), channel))
	}

	if !install {
		return writeOut(cmd, fmt.Appendf(nil,
			"cwm %s is available; you have %s. Run \"cwm update\" to install it.\n",
			release.Version(), info.Short(),
		))
	}

	target, err := updater.Apply(cmd.Context(), release)
	if err != nil {
		return fmt.Errorf("install cwm %s: %w", release.Version(), err)
	}

	return writeOut(cmd, fmt.Appendf(nil, "updated cwm to %s at %s\n", release.Version(), target))
}

// autoUpdate is the check that runs before an unrelated command.
//
// It never returns an error. Someone running "cwm config show" asked about
// their config, and a rate limit or a flaky network must not turn that into a
// failure.
func autoUpdate(cmd *cobra.Command, info version.Info, resolver *paths.Resolver, newUpdater updaterFactory) {
	if skipsUpdateCheck(cmd) {
		return
	}

	updater, err := newUpdater()
	if err != nil || !updater.Updatable() || !updater.Due() {
		return
	}

	cfg, err := loadConfig(cmd, resolver)
	if err != nil {
		return
	}

	// Recorded before the check, so that a machine with no network does not try
	// again on every single command.
	if markErr := updater.MarkChecked(); markErr != nil {
		return
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), autoCheckTimeout)
	defer cancel()

	release, found, err := updater.Check(ctx, cfg.Update.Channel)
	if err != nil || !found {
		return
	}

	if cfg.Update.Mode == config.ModeCheck {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"cwm %s is available; you have %s. Run \"cwm update\" to install it.\n",
			release.Version(), info.Short(),
		)

		return
	}

	target, err := updater.Apply(ctx, release)
	if err != nil {
		// Worth one line: the user asked for automatic updates and is not
		// getting them, and the usual cause is a binary they cannot write to.
		fmt.Fprintf(cmd.ErrOrStderr(), "cwm could not install %s: %v\n", release.Version(), err)

		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "cwm updated itself to %s at %s\n", release.Version(), target)
}

// skipsUpdateCheck reports whether cmd is one the automatic check stays out of.
//
// The update commands do the check themselves, and the machine-facing commands
// must not have their output polluted by a nag.
func skipsUpdateCheck(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case updateCmdName, "completion", "help",
			cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		default:
		}
	}

	return false
}

// updateChannel returns the channel to use, honouring --pre over the setting.
func updateChannel(cmd *cobra.Command, resolver *paths.Resolver, prerelease bool) (config.Channel, error) {
	if prerelease {
		return config.ChannelPrerelease, nil
	}

	cfg, err := loadConfig(cmd, resolver)
	if err != nil {
		return "", err
	}

	return cfg.Update.Channel, nil
}

// loadConfig reads the configuration the update commands consult.
func loadConfig(cmd *cobra.Command, resolver *paths.Resolver) (config.Config, error) {
	store, err := newConfigStore(resolver)
	if err != nil {
		return config.Config{}, err
	}

	cfg, err := store.Load(cmd.Context())
	if err != nil {
		return config.Config{}, fmt.Errorf("load the configuration: %w", err)
	}

	return cfg, nil
}

// newOSUpdater builds the updater the real commands use.
func newOSUpdater(resolver *paths.Resolver, info version.Info) (Updater, error) {
	root, err := resolver.CacheRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve the cache root: %w", err)
	}

	client, err := update.NewClient(update.ClientOptions{
		HTTPClient: &http.Client{Timeout: downloadTimeout},
		BaseURL:    "",
		Repo:       update.DefaultRepo,
		UserAgent:  userAgent(info),
	})
	if err != nil {
		return nil, fmt.Errorf("build the github client: %w", err)
	}

	return update.NewUpdater(client, cache.New(root), info.Short(), time.Now), nil
}

// userAgent identifies this cwm to GitHub, which requires one.
func userAgent(info version.Info) string {
	return "cwm/" + info.Short() + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
}
