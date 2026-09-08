package update

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/pdylanross/claude-workspace-manager/internal/cache"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// CacheEntry records when cwm last looked for a newer release.
const CacheEntry = "last_update"

// CheckInterval is how long cwm waits between automatic checks. Looking once a
// day is often enough to matter and rare enough that nobody notices.
const CheckInterval = 24 * time.Hour

// Updater decides when to look for a newer cwm, finds one, and installs it.
type Updater struct {
	client  *Client
	entries *cache.Cache
	current string
	now     func() time.Time
}

// NewUpdater returns an Updater for the running version. now supplies the
// clock, so that the interval can be tested without waiting a day.
func NewUpdater(client *Client, entries *cache.Cache, current string, now func() time.Time) *Updater {
	return &Updater{client: client, entries: entries, current: current, now: now}
}

// Updatable reports whether the running build is one that can be updated.
//
// A "go build" binary and a goreleaser snapshot both say a version with no
// release behind it; replacing either with a published build would throw away
// whatever the developer was testing.
func (u *Updater) Updatable() bool {
	return IsRelease(u.current)
}

// Due reports whether enough time has passed since the last check.
//
// Never having checked is due. So is a cache entry cwm cannot read: the point
// of the entry is to skip work, and when in doubt the safe answer is to do it.
func (u *Updater) Due() bool {
	last, found, err := u.entries.ReadTime(CacheEntry)
	if err != nil || !found {
		return true
	}

	return u.now().Sub(last) >= CheckInterval
}

// MarkChecked records that a check happened now.
//
// Callers record this before the check rather than after, so that a run with no
// network does not retry on every single command.
func (u *Updater) MarkChecked() error {
	if err := u.entries.WriteTime(CacheEntry, u.now()); err != nil {
		return fmt.Errorf("record the update check: %w", err)
	}

	return nil
}

// Check returns the newest release on channel that is newer than the running
// version, if there is one. It writes nothing.
func (u *Updater) Check(ctx context.Context, channel config.Channel) (Release, bool, error) {
	releases, err := u.client.Releases(ctx)
	if err != nil {
		return Release{}, false, err
	}

	release, found := Newer(releases, u.current, channel)

	return release, found, nil
}

// Apply downloads a release and replaces the running binary with it, returning
// where it was installed.
//
// The archive is checked against the digest published alongside it before
// anything is unpacked. cwm is about to make this file the program the user
// runs, so an archive that cannot be verified is refused rather than installed
// with a warning.
func (u *Updater) Apply(ctx context.Context, release Release) (string, error) {
	target, err := executablePath()
	if err != nil {
		return "", err
	}

	return u.applyTo(ctx, release, target)
}

// applyTo is Apply against an explicit target, so that everything but the
// one-line lookup of the running binary can be exercised without a test
// overwriting the test binary.
func (u *Updater) applyTo(ctx context.Context, release Release, target string) (string, error) {
	asset, found := release.PlatformAsset()
	if !found {
		return "", fmt.Errorf("%w: %s has no build for %s/%s",
			ErrNoAsset, release.Version(), runtime.GOOS, runtime.GOARCH)
	}

	sums, found := release.Asset(ChecksumsAsset)
	if !found {
		return "", fmt.Errorf("%w: %s publishes no %s", ErrNoChecksum, release.Version(), ChecksumsAsset)
	}

	archive, err := u.client.Archive(ctx, asset)
	if err != nil {
		return "", err
	}

	checksums, err := u.client.Checksums(ctx, sums)
	if err != nil {
		return "", err
	}

	if verifyErr := verifyChecksum(archive, checksums, asset.Name); verifyErr != nil {
		return "", verifyErr
	}

	binary, err := extractBinary(archive)
	if err != nil {
		return "", err
	}

	if replaceErr := replaceExecutable(binary, target); replaceErr != nil {
		return "", replaceErr
	}

	return target, nil
}
