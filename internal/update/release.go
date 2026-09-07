// Package update keeps cwm current with its own GitHub releases.
//
// The flow is deliberately split: [Updater.Due] answers whether it is time to
// look, [Updater.Check] finds a newer release without touching anything, and
// [Updater.Apply] is the only part that writes. A caller that only wants to nag
// never reaches the third.
package update

import (
	"errors"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// ChecksumsAsset is the release asset goreleaser publishes the digests in.
const ChecksumsAsset = "checksums.txt"

// snapshotMarker appears in the version goreleaser stamps into a snapshot
// build. Such a build is a local artefact with no release behind it.
const snapshotMarker = "SNAPSHOT"

// ErrNoAsset is returned when a release has no build for this platform, which
// means cwm cannot update itself from it even though it is newer.
var ErrNoAsset = errors.New("no release asset for this platform")

// Release is one published cwm release.
type Release struct {
	// Tag is the git tag, with its leading "v".
	Tag string
	// Prerelease reports whether the release is a prerelease.
	Prerelease bool
	// Assets are the files published with the release.
	Assets []Asset
}

// Asset is one file published with a release.
type Asset struct {
	// Name is the file name, as in cwm_1.2.3_linux_amd64.tar.gz.
	Name string
	// URL is where the file can be downloaded from.
	URL string
}

// IsRelease reports whether version is one this build can compare against a
// published release.
//
// A build from "go build" says "dev" and a goreleaser snapshot says something
// like 0.0.0-SNAPSHOT-7ed3322. Neither has a release behind it, and neither
// should be silently replaced by one, so cwm treats both as not updatable.
func IsRelease(version string) bool {
	if strings.Contains(version, snapshotMarker) {
		return false
	}

	_, ok := canonical(version)

	return ok
}

// Newer returns the release that cwm should move to, out of releases, given the
// version running now and the channel the user asked for.
//
// Releases are compared by semantic version rather than by publication order,
// so a patch published for an older line after a newer one does not drag a user
// backwards.
func Newer(releases []Release, current string, channel config.Channel) (Release, bool) {
	currentVersion, ok := canonical(current)
	if !ok {
		return Release{}, false
	}

	var (
		best        Release
		bestVersion string
		found       bool
	)

	for _, release := range releases {
		version, valid := canonical(release.Tag)
		if !valid || !release.eligible(channel) {
			continue
		}

		if semver.Compare(version, currentVersion) <= 0 {
			continue
		}

		if found && semver.Compare(version, bestVersion) <= 0 {
			continue
		}

		best, bestVersion, found = release, version, true
	}

	return best, found
}

// AssetName returns the file name of the build for this platform, following
// goreleaser's archive naming.
func AssetName(tag string) string {
	return "cwm_" + strings.TrimPrefix(tag, "v") + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
}

// Version returns the release's version without the leading "v", the form a
// user sees.
func (r Release) Version() string {
	return strings.TrimPrefix(r.Tag, "v")
}

// Asset returns the asset published under name.
func (r Release) Asset(name string) (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}

	return Asset{}, false
}

// PlatformAsset returns the build for the running platform.
//
// It matches on the platform suffix rather than the whole name, so that a
// release whose version string is written differently from the tag still
// resolves.
func (r Release) PlatformAsset() (Asset, bool) {
	suffix := "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"

	for _, asset := range r.Assets {
		if strings.HasSuffix(asset.Name, suffix) {
			return asset, true
		}
	}

	return Asset{}, false
}

// eligible reports whether the release may be offered on channel.
func (r Release) eligible(channel config.Channel) bool {
	if channel == config.ChannelPrerelease {
		return true
	}

	return !r.isPrerelease()
}

// isPrerelease reports whether the release is a prerelease according to either
// GitHub or the tag itself.
//
// Both are consulted because they can disagree: goreleaser marks the release
// from the tag, but a release can also be flagged by hand in the GitHub UI, and
// a tag can carry a prerelease suffix that nobody flagged.
func (r Release) isPrerelease() bool {
	if r.Prerelease {
		return true
	}

	version, ok := canonical(r.Tag)

	return ok && semver.Prerelease(version) != ""
}

// canonical turns a version or tag into a comparable semver string, reporting
// false for anything that is not one.
func canonical(version string) (string, bool) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return "", false
	}

	if !strings.HasPrefix(trimmed, "v") {
		trimmed = "v" + trimmed
	}

	if !semver.IsValid(trimmed) {
		return "", false
	}

	return trimmed, true
}
