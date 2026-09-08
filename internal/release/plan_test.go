package release_test

import (
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/release"
)

// Deliberately not parallel, at either level: each step is the state the next
// one starts from, which is the whole point of replaying the cycle in order.
func TestNextWalksThroughAReleaseCycle(t *testing.T) {
	// The worked example this design was specified with, replayed in order.
	// Each step adds a commit to main and asserts the tag that gets cut, so the
	// counter restarting when the target moves is checked where it happens
	// rather than in isolation.
	steps := []struct {
		name string
		// commit is added to those since the last real release.
		commit string
		// promoted, when set, is a real release cut at this point, which resets
		// what later steps measure against.
		promoted string
		want     string
	}{
		{
			name:   "a fix after 1.0.0 targets 1.0.1",
			commit: "fix: something",
			want:   "v1.0.1-pre1",
		},
		{
			name:   "a second fix keeps the target and counts on",
			commit: "fix: something else",
			want:   "v1.0.1-pre2",
		},
		{
			name:     "a fix after 1.0.1 was promoted starts again",
			commit:   "fix: something else",
			promoted: "v1.0.1",
			want:     "v1.0.2-pre1",
		},
		{
			name:   "a feature retargets to 1.1.0 and the counter restarts",
			commit: "feat: some feat",
			want:   "v1.1.0-pre1",
		},
		{
			name:   "a later fix does not pull the target back down",
			commit: "fix: another",
			want:   "v1.1.0-pre2",
		},
		{
			name:   "a second feature does not move it either",
			commit: "feat: another feat",
			want:   "v1.1.0-pre3",
		},
	}

	tags := []string{"v1.0.0"}
	previous := release.Version{Major: 1, Minor: 0, Patch: 0}

	var messages []string

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			if step.promoted != "" {
				promoted, err := release.ParseVersion(step.promoted)
				if err != nil {
					t.Fatalf("ParseVersion() error = %v", err)
				}

				tags = append(tags, step.promoted)
				previous = promoted
				messages = nil
			}

			messages = append(messages, step.commit)

			plan, ok := release.Next(previous, messages, tags)
			if !ok {
				t.Fatal("Next() found nothing to cut, want a prerelease")
			}

			if got := plan.Prerelease.Tag(); got != step.want {
				t.Fatalf("Next() = %s, want %s", got, step.want)
			}

			tags = append(tags, plan.Prerelease.Tag())
		})
	}

	// The tag abandoned when the target moved is still there and still ignored.
	if _, err := release.ParsePrerelease("v1.0.2-pre1"); err != nil {
		t.Errorf("the abandoned tag should still parse: %v", err)
	}
}

func TestNextCutsNothingWithoutAReleasableCommit(t *testing.T) {
	t.Parallel()

	previous := release.Version{Major: 1, Minor: 0, Patch: 0}
	messages := []string{"docs: explain the thing", "chore: bump a dependency", "chore(ci): tidy"}

	if plan, ok := release.Next(previous, messages, []string{"v1.0.0"}); ok {
		t.Errorf("Next() = %s, want nothing cut for housekeeping commits", plan.Prerelease.Tag())
	}
}

func TestNextFromNothing(t *testing.T) {
	t.Parallel()

	// A repository with no releases at all starts at zero, so the first fix
	// cuts 0.0.1-pre1 rather than inventing a 1.0.0.
	plan, ok := release.Next(release.LatestRelease(nil), []string{"fix: the first thing"}, nil)
	if !ok {
		t.Fatal("Next() found nothing to cut")
	}

	if got := plan.Prerelease.Tag(); got != "v0.0.1-pre1" {
		t.Errorf("Next() = %s, want v0.0.1-pre1", got)
	}
}

func TestNextTakesTheLargestBump(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []string
		want     string
	}{
		{"a fix alone", []string{"fix: a"}, "v1.2.4-pre1"},
		{"a feature outranks fixes", []string{"fix: a", "feat: b", "fix: c"}, "v1.3.0-pre1"},
		{"a breaking change outranks everything", []string{"fix: a", "feat!: b", "feat: c"}, "v2.0.0-pre1"},
		{"the footer counts too", []string{"refactor: a\n\nBREAKING CHANGE: it moved"}, "v2.0.0-pre1"},
		{"perf is a patch", []string{"perf: faster"}, "v1.2.4-pre1"},
	}

	previous := release.Version{Major: 1, Minor: 2, Patch: 3}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, ok := release.Next(previous, tt.messages, []string{"v1.2.3"})
			if !ok {
				t.Fatal("Next() found nothing to cut")
			}

			if got := plan.Prerelease.Tag(); got != tt.want {
				t.Errorf("Next() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNextCountsPastTenPrereleases(t *testing.T) {
	t.Parallel()

	tags := []string{"v1.0.0"}
	for number := 1; number <= 11; number++ {
		prerelease := release.Prerelease{Target: release.Version{Major: 1, Minor: 0, Patch: 1}, Number: number}
		tags = append(tags, prerelease.Tag())
	}

	// The counter is compared as a number, so pre11 is the highest rather than
	// pre9 winning on text order.
	plan, ok := release.Next(release.Version{Major: 1, Minor: 0, Patch: 0}, []string{"fix: a"}, tags)
	if !ok {
		t.Fatal("Next() found nothing to cut")
	}

	if got := plan.Prerelease.Tag(); got != "v1.0.1-pre12" {
		t.Errorf("Next() = %s, want v1.0.1-pre12", got)
	}
}

func TestLatestRelease(t *testing.T) {
	t.Parallel()

	tags := []string{"v1.0.0", "v1.2.0", "v1.10.0", "v1.9.0", "v1.11.0-pre3", "not-a-tag", "v2.0.0-pre1"}

	// 1.10.0 beats 1.9.0 numerically, and neither prerelease counts.
	if got := release.LatestRelease(tags); got.Tag() != "v1.10.0" {
		t.Errorf("LatestRelease() = %s, want v1.10.0", got.Tag())
	}
}

func TestLatestPrerelease(t *testing.T) {
	t.Parallel()

	tags := []string{"v1.0.0", "v1.0.2-pre1", "v1.1.0-pre1", "v1.1.0-pre10", "v1.1.0-pre9"}

	latest, found := release.LatestPrerelease(tags)
	if !found {
		t.Fatal("LatestPrerelease() found nothing")
	}

	if got := latest.Tag(); got != "v1.1.0-pre10" {
		t.Errorf("LatestPrerelease() = %s, want v1.1.0-pre10", got)
	}
}

func TestPromotable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tags  []string
		want  string
		found bool
	}{
		{
			name:  "the newest prerelease is what gets promoted",
			tags:  []string{"v1.0.0", "v1.0.2-pre1", "v1.1.0-pre1", "v1.1.0-pre3", "v1.1.0-pre2"},
			want:  "v1.1.0",
			found: true,
		},
		{
			name:  "nothing to promote without a prerelease",
			tags:  []string{"v1.0.0", "v1.1.0"},
			found: false,
		},
		{
			name:  "a prerelease whose target is already out is not promotable",
			tags:  []string{"v1.0.0", "v1.1.0-pre1", "v1.1.0"},
			found: false,
		},
		{
			name:  "nothing to promote in an empty repository",
			tags:  nil,
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, found := release.Promotable(tt.tags)
			if found != tt.found {
				t.Fatalf("Promotable() found = %t, want %t", found, tt.found)
			}

			if found && version.Tag() != tt.want {
				t.Errorf("Promotable() = %s, want %s", version.Tag(), tt.want)
			}
		})
	}
}
