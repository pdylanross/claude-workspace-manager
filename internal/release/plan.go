package release

import "slices"

// Plan is what a push to main should cut.
type Plan struct {
	// Previous is the release the commits were measured against.
	Previous Version
	// Bump is what those commits ask for.
	Bump Bump
	// Prerelease is the tag to cut.
	Prerelease Prerelease
}

// Next decides which prerelease to cut, given the last real release, the
// commits made since it, and every tag in the repository.
//
// It reports false when the commits ask for nothing, which is the normal
// outcome of a docs or chore push: main moves without a release.
//
// The prerelease number counts within the target version rather than overall.
// That is what makes the counter restart when the target moves: cutting
// 1.0.2-pre1 for a fix and then taking a feature retargets to 1.1.0, whose
// counter has never been used, so the next cut is 1.1.0-pre1 and the 1.0.2-pre1
// tag is simply left behind.
func Next(previous Version, messages []string, tags []string) (Plan, bool) {
	bump := BumpForAll(messages)
	if bump == BumpNone {
		return Plan{}, false
	}

	target := previous.Apply(bump)

	return Plan{
		Previous:   previous,
		Bump:       bump,
		Prerelease: Prerelease{Target: target, Number: nextNumber(target, tags)},
	}, true
}

// LatestRelease returns the newest real release among tags.
//
// The zero version when there is none, which is the right starting point: a
// repository with no releases and one fix commit cuts 0.0.1-pre1.
func LatestRelease(tags []string) Version {
	var latest Version

	for _, tag := range tags {
		version, err := ParseVersion(tag)
		if err != nil {
			continue
		}

		if version.Compare(latest) > 0 {
			latest = version
		}
	}

	return latest
}

// LatestPrerelease returns the newest prerelease among tags, which is the one a
// promotion turns into a real release.
func LatestPrerelease(tags []string) (Prerelease, bool) {
	var (
		latest Prerelease
		found  bool
	)

	for _, tag := range tags {
		prerelease, err := ParsePrerelease(tag)
		if err != nil {
			continue
		}

		if !found || prerelease.Compare(latest) > 0 {
			latest, found = prerelease, true
		}
	}

	return latest, found
}

// Promotable returns the release a promotion would cut, and whether there is
// one to cut.
//
// A prerelease whose target has already been released is not promotable: that
// is a leftover from a target the commits moved past, and promoting it would
// re-release something that already exists.
func Promotable(tags []string) (Version, bool) {
	prerelease, found := LatestPrerelease(tags)
	if !found {
		return Version{}, false
	}

	if slices.Contains(releaseTags(tags), prerelease.Target.Tag()) {
		return Version{}, false
	}

	return prerelease.Target, true
}

// nextNumber returns the prerelease number to use for target: one past the
// highest already cut for it, and 1 when none has been.
func nextNumber(target Version, tags []string) int {
	highest := 0

	for _, tag := range tags {
		prerelease, err := ParsePrerelease(tag)
		if err != nil || prerelease.Target.Compare(target) != 0 {
			continue
		}

		if prerelease.Number > highest {
			highest = prerelease.Number
		}
	}

	return highest + 1
}

// releaseTags returns the tags that name a real release.
func releaseTags(tags []string) []string {
	released := make([]string, 0, len(tags))

	for _, tag := range tags {
		version, err := ParseVersion(tag)
		if err != nil {
			continue
		}

		released = append(released, version.Tag())
	}

	return released
}
