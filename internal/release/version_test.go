package release_test

import (
	"errors"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/release"
)

func TestBumpFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    release.Bump
	}{
		{"a fix", "fix: something", release.BumpPatch},
		{"a scoped fix", "fix(config): something", release.BumpPatch},
		{"a feature", "feat: something", release.BumpMinor},
		{"a scoped feature", "feat(cli): something", release.BumpMinor},
		{"a performance change", "perf: faster", release.BumpPatch},
		{"a breaking feature", "feat!: something", release.BumpMajor},
		{"a breaking scoped fix", "fix(api)!: something", release.BumpMajor},
		{"a breaking chore", "chore!: drop the thing", release.BumpMajor},
		{"the breaking footer", "refactor: move it\n\nBREAKING CHANGE: the flag is gone", release.BumpMajor},
		{"the hyphenated footer", "refactor: move it\n\nBREAKING-CHANGE: the flag is gone", release.BumpMajor},
		{"documentation", "docs: explain", release.BumpNone},
		{"a chore", "chore: tidy", release.BumpNone},
		{"a test", "test: cover it", release.BumpNone},
		{"a refactor", "refactor: rearrange", release.BumpNone},
		{"an unlabelled commit", "just did some stuff", release.BumpNone},
		{"an empty message", "", release.BumpNone},
		{"uppercase still counts", "FIX: something", release.BumpPatch},
		{"the word breaking in prose does not count", "fix: stop breaking change detection", release.BumpPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := release.BumpFor(tt.message); got != tt.want {
				t.Errorf("BumpFor(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestBumpForAll(t *testing.T) {
	t.Parallel()

	if got := release.BumpForAll(nil); got != release.BumpNone {
		t.Errorf("BumpForAll(nil) = %v, want BumpNone", got)
	}

	messages := []string{"docs: a", "fix: b", "chore: c"}
	if got := release.BumpForAll(messages); got != release.BumpPatch {
		t.Errorf("BumpForAll() = %v, want BumpPatch", got)
	}
}

func TestVersionApply(t *testing.T) {
	t.Parallel()

	previous := release.Version{Major: 1, Minor: 2, Patch: 3}

	tests := []struct {
		name string
		bump release.Bump
		want string
	}{
		{"a patch moves the last number", release.BumpPatch, "1.2.4"},
		{"a minor zeroes the patch", release.BumpMinor, "1.3.0"},
		{"a major zeroes the rest", release.BumpMajor, "2.0.0"},
		{"nothing stays put", release.BumpNone, "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := previous.Apply(tt.bump).String(); got != tt.want {
				t.Errorf("Apply(%v) = %s, want %s", tt.bump, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  string
		want string
	}{
		{"with the v", "v1.2.3", "v1.2.3"},
		{"without the v", "1.2.3", "v1.2.3"},
		{"double figures", "v1.10.20", "v1.10.20"},
		{"surrounding space", "  v1.2.3\n", "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, err := release.ParseVersion(tt.tag)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", tt.tag, err)
			}

			if got := version.Tag(); got != tt.want {
				t.Errorf("ParseVersion(%q) = %s, want %s", tt.tag, got, tt.want)
			}
		})
	}
}

func TestParseVersionRejectsOtherTags(t *testing.T) {
	t.Parallel()

	// A prerelease is deliberately not a version: it is what ParsePrerelease is
	// for, and treating it as a release would let a promotion pick it up.
	for _, tag := range []string{"", "v1.2", "v1.2.3.4", "v1.2.3-pre1", "release-1", "v1.2.3+build"} {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			if _, err := release.ParseVersion(tag); !errors.Is(err, release.ErrNotAVersion) {
				t.Errorf("ParseVersion(%q) error = %v, want ErrNotAVersion", tag, err)
			}
		})
	}
}

func TestParsePrerelease(t *testing.T) {
	t.Parallel()

	prerelease, err := release.ParsePrerelease("v1.0.1-pre12")
	if err != nil {
		t.Fatalf("ParsePrerelease() error = %v", err)
	}

	if prerelease.Target.Tag() != "v1.0.1" || prerelease.Number != 12 {
		t.Errorf("ParsePrerelease() = %+v, want target v1.0.1 number 12", prerelease)
	}

	if got := prerelease.Tag(); got != "v1.0.1-pre12" {
		t.Errorf("Tag() = %s, want v1.0.1-pre12", got)
	}
}

func TestParsePrereleaseRejectsOtherTags(t *testing.T) {
	t.Parallel()

	for _, tag := range []string{"v1.0.1", "v1.0.1-rc1", "v1.0.1-pre", "v1.0.1-pre1.2", ""} {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			if _, err := release.ParsePrerelease(tag); !errors.Is(err, release.ErrNotAVersion) {
				t.Errorf("ParsePrerelease(%q) error = %v, want ErrNotAVersion", tag, err)
			}
		})
	}
}

func TestPrereleaseCompare(t *testing.T) {
	t.Parallel()

	older := release.Prerelease{Target: release.Version{Major: 1, Minor: 0, Patch: 1}, Number: 2}
	newer := release.Prerelease{Target: release.Version{Major: 1, Minor: 0, Patch: 1}, Number: 10}

	// pre10 is newer than pre2, which is the opposite of what comparing the
	// tags as text would say.
	if older.Compare(newer) >= 0 {
		t.Errorf("Compare(%s, %s) said the first is not older", older.Tag(), newer.Tag())
	}

	retargeted := release.Prerelease{Target: release.Version{Major: 1, Minor: 1, Patch: 0}, Number: 1}
	if newer.Compare(retargeted) >= 0 {
		t.Errorf("Compare(%s, %s) ignored the target", newer.Tag(), retargeted.Tag())
	}
}
