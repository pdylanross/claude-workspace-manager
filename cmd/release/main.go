// Command release works out what the CI pipeline should tag next.
//
// It is release tooling, not part of cwm: goreleaser builds ./cmd/cwm alone, so
// nothing here ships in the binary a user installs.
//
// Usage:
//
//	release plan       what a push to main should cut, or nothing
//	release promote    the release the newest prerelease would be promoted to
//
// Both write "key=value" lines to stdout and their reasoning to stderr, so a
// workflow step can append stdout straight to $GITHUB_OUTPUT and read the keys
// back. Writing nothing means there is nothing to do, which is a success rather
// than a failure: a docs-only push releases nothing, and the workflow skips the
// steps whose input is empty.
//
//	plan     tag=v1.1.0-pre3
//	promote  tag=v1.1.0
//	         source=v1.1.0-pre3
//
// promote reports the source as well as the target because main may have moved
// while the promotion waited for approval: the release has to be cut at the
// commit that was actually tested, not at whatever HEAD has become.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pdylanross/claude-workspace-manager/internal/release"
)

// commitSeparator ends each commit in the log, so that a message spanning
// several lines stays one entry.
//
// git writes it from the %x00 escape in the format. The escape is what makes
// this work: a NUL cannot be passed to git as part of an argument, because
// arguments are NUL-terminated themselves.
const (
	commitSeparator = "\x00"
	separatorEscape = "%x00"
)

// gitTimeout bounds every git command this tool runs, together.
const gitTimeout = 2 * time.Minute

func main() {
	os.Exit(exitCode())
}

// exitCode runs the tool and reports the status to exit with.
//
// main does nothing but exit, so that the context's cancel actually runs: a
// deferred call in main would be skipped by [os.Exit].
func exitCode() int {
	// A tool that shells out to git deserves a ceiling: a hung git in CI should
	// fail the step rather than hold a runner until the job times out.
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)

		return 1
	}

	return 0
}

// run dispatches a subcommand.
func run(ctx context.Context, args []string, out, explain *os.File) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one of \"plan\" or \"promote\", got %v", args)
	}

	tags, err := gitTags(ctx)
	if err != nil {
		return err
	}

	switch args[0] {
	case "plan":
		return plan(ctx, tags, out, explain)
	case "promote":
		return promote(tags, out, explain)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// plan prints the prerelease tag a push to main should cut.
func plan(ctx context.Context, tags []string, out, explain *os.File) error {
	previous := release.LatestRelease(tags)

	messages, err := commitsSince(ctx, previous, tags)
	if err != nil {
		return err
	}

	decision, ok := release.Next(previous, messages, tags)
	if !ok {
		fmt.Fprintf(explain, "no releasable commits since %s; nothing to cut\n", previous.Tag())

		return nil
	}

	fmt.Fprintf(explain, "%d commits since %s ask for a %s release\n",
		len(messages), previous.Tag(), bumpName(decision.Bump))
	fmt.Fprintf(out, "tag=%s\n", decision.Prerelease.Tag())

	return nil
}

// promote prints the release tag the newest prerelease should become.
func promote(tags []string, out, explain *os.File) error {
	version, ok := release.Promotable(tags)
	if !ok {
		fmt.Fprintln(explain, "no prerelease is waiting to be promoted")

		return nil
	}

	prerelease, _ := release.LatestPrerelease(tags)

	fmt.Fprintf(explain, "promoting %s to %s\n", prerelease.Tag(), version.Tag())
	fmt.Fprintf(out, "tag=%s\n", version.Tag())
	fmt.Fprintf(out, "source=%s\n", prerelease.Tag())

	return nil
}

// commitsSince returns the messages of the commits made since a release.
//
// When that release has no tag in the repository — which is what the zero
// version means — the whole history counts.
func commitsSince(ctx context.Context, previous release.Version, tags []string) ([]string, error) {
	revisions := "HEAD"
	for _, tag := range tags {
		if tag == previous.Tag() {
			revisions = previous.Tag() + "..HEAD"

			break
		}
	}

	// %B is the raw body, so the breaking-change footer survives; the NUL
	// separator keeps a multi-line message in one piece.
	output, err := git(ctx, "log", revisions, "--format=%B"+separatorEscape)
	if err != nil {
		return nil, err
	}

	messages := make([]string, 0)

	for message := range strings.SplitSeq(output, commitSeparator) {
		if trimmed := strings.TrimSpace(message); trimmed != "" {
			messages = append(messages, trimmed)
		}
	}

	return messages, nil
}

// gitTags returns every tag in the repository.
func gitTags(ctx context.Context) ([]string, error) {
	output, err := git(ctx, "tag", "--list")
	if err != nil {
		return nil, err
	}

	return strings.Fields(output), nil
}

// git runs a git command and returns its output.
func git(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)

	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return string(output), nil
}

// bumpName describes a bump for the log.
func bumpName(bump release.Bump) string {
	switch bump {
	case release.BumpMajor:
		return "major"
	case release.BumpMinor:
		return "minor"
	case release.BumpPatch:
		return "patch"
	case release.BumpNone:
		return "no"
	default:
		return "no"
	}
}
