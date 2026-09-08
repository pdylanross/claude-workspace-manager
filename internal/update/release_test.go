package update_test

import (
	"runtime"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/update"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// platformAsset names the archive for the platform the tests run on.
func platformAsset(version string) string {
	return "cwm_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
}

// releaseOf builds a release carrying the assets goreleaser publishes.
func releaseOf(tag string, prerelease bool) update.Release {
	version := tag[1:]

	return update.Release{
		Tag:        tag,
		Prerelease: prerelease,
		Assets: []update.Asset{
			{Name: "cwm_" + version + "_linux_amd64.tar.gz", ID: 1},
			{Name: "cwm_" + version + "_darwin_arm64.tar.gz", ID: 2},
			{Name: platformAsset(version), ID: 3},
			{Name: update.ChecksumsAsset, ID: 4},
		},
	}
}

func TestIsRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"a stamped release", "1.2.3", true},
		{"a tag with its v", "v1.2.3", true},
		{"a prerelease", "1.2.3-pre.1", true},
		{"a plain go build", "dev", false},
		{"a goreleaser snapshot", "0.0.0-SNAPSHOT-7ed3322", false},
		{"nothing at all", "", false},
		{"not a version", "banana", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := update.IsRelease(tt.version); got != tt.want {
				t.Errorf("IsRelease(%q) = %t, want %t", tt.version, got, tt.want)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()

	releases := []update.Release{
		releaseOf("v1.0.0", false),
		releaseOf("v1.2.0", false),
		releaseOf("v1.3.0-pre.1", true),
		releaseOf("v1.1.0", false),
	}

	tests := []struct {
		name    string
		current string
		channel config.Channel
		wantTag string
		wantAny bool
	}{
		{
			name:    "stable takes the newest full release",
			current: "1.0.0",
			channel: config.ChannelStable,
			wantTag: "v1.2.0",
			wantAny: true,
		},
		{
			name:    "prerelease takes the newest of either kind",
			current: "1.0.0",
			channel: config.ChannelPrerelease,
			wantTag: "v1.3.0-pre.1",
			wantAny: true,
		},
		{
			name:    "nothing newer on stable",
			current: "1.2.0",
			channel: config.ChannelStable,
			wantAny: false,
		},
		{
			name:    "a prerelease does not tempt a stable user",
			current: "1.2.0",
			channel: config.ChannelPrerelease,
			wantTag: "v1.3.0-pre.1",
			wantAny: true,
		},
		{
			name:    "running something newer than anything published",
			current: "2.0.0",
			channel: config.ChannelPrerelease,
			wantAny: false,
		},
		{
			name:    "an unreadable current version offers nothing",
			current: "dev",
			channel: config.ChannelStable,
			wantAny: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := update.Newer(releases, tt.current, tt.channel)
			if found != tt.wantAny {
				t.Fatalf("Newer() found = %t, want %t (got %q)", found, tt.wantAny, got.Tag)
			}

			if found && got.Tag != tt.wantTag {
				t.Errorf("Newer() = %q, want %q", got.Tag, tt.wantTag)
			}
		})
	}
}

func TestNewerComparesBySemverNotByOrder(t *testing.T) {
	t.Parallel()

	// A patch on an older line, published after the newer line, must not drag
	// anyone backwards.
	releases := []update.Release{
		releaseOf("v2.0.0", false),
		releaseOf("v1.9.9", false),
	}

	got, found := update.Newer(releases, "1.0.0", config.ChannelStable)
	if !found || got.Tag != "v2.0.0" {
		t.Errorf("Newer() = %q, %t, want v2.0.0, true", got.Tag, found)
	}
}

func TestNewerIgnoresUnreadableTags(t *testing.T) {
	t.Parallel()

	releases := []update.Release{
		{Tag: "nightly", Prerelease: false, Assets: nil},
		releaseOf("v1.1.0", false),
	}

	got, found := update.Newer(releases, "1.0.0", config.ChannelStable)
	if !found || got.Tag != "v1.1.0" {
		t.Errorf("Newer() = %q, %t, want v1.1.0, true", got.Tag, found)
	}
}

func TestNewerTreatsATaggedPrereleaseAsOne(t *testing.T) {
	t.Parallel()

	// GitHub's flag says no, but the tag says yes; the tag has to win, or a
	// stable user gets a prerelease.
	releases := []update.Release{releaseOf("v1.5.0-rc.1", false)}

	if _, found := update.Newer(releases, "1.0.0", config.ChannelStable); found {
		t.Error("Newer() offered a prerelease tag on the stable channel")
	}

	if _, found := update.Newer(releases, "1.0.0", config.ChannelPrerelease); !found {
		t.Error("Newer() withheld a prerelease tag on the prerelease channel")
	}
}

func TestNewerTreatsAFlaggedReleaseAsAPrerelease(t *testing.T) {
	t.Parallel()

	// The other direction: a plain tag flagged by hand in the GitHub UI.
	releases := []update.Release{releaseOf("v1.5.0", true)}

	if _, found := update.Newer(releases, "1.0.0", config.ChannelStable); found {
		t.Error("Newer() offered a flagged prerelease on the stable channel")
	}
}

func TestReleaseAssets(t *testing.T) {
	t.Parallel()

	release := releaseOf("v1.2.3", false)

	if got := release.Version(); got != "1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "1.2.3")
	}

	asset, found := release.PlatformAsset()
	if !found {
		t.Fatal("PlatformAsset() found nothing for this platform")
	}

	if asset.Name != platformAsset("1.2.3") {
		t.Errorf("PlatformAsset() = %q, want %q", asset.Name, platformAsset("1.2.3"))
	}

	if _, ok := release.Asset(update.ChecksumsAsset); !ok {
		t.Error("Asset(checksums.txt) found nothing")
	}

	if _, ok := release.Asset("nothing.tar.gz"); ok {
		t.Error("Asset() found an asset that is not there")
	}
}

func TestPlatformAssetMissing(t *testing.T) {
	t.Parallel()

	release := update.Release{
		Tag:        "v1.2.3",
		Prerelease: false,
		Assets:     []update.Asset{{Name: "cwm_1.2.3_plan9_mips.tar.gz", ID: 9}},
	}

	if _, found := release.PlatformAsset(); found {
		t.Error("PlatformAsset() found a build for a platform that is not this one")
	}
}

func TestAssetName(t *testing.T) {
	t.Parallel()

	if got := update.AssetName("v1.2.3"); got != platformAsset("1.2.3") {
		t.Errorf("AssetName() = %q, want %q", got, platformAsset("1.2.3"))
	}

	if got := update.AssetName("1.2.3"); got != platformAsset("1.2.3") {
		t.Errorf("AssetName() without the v = %q, want %q", got, platformAsset("1.2.3"))
	}
}
