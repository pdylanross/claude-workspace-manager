// Package release works out what to release next from the commits since the
// last release.
//
// It is deliberately pure: everything here takes tags and commit messages as
// data and returns a decision. Reading git is [github.com/pdylanross/claude-workspace-manager/cmd/release]'s
// job, so the rules can be tested without a repository.
package release

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PrereleaseLabel is the identifier cut prereleases carry, as in v1.0.1-pre1.
const PrereleaseLabel = "pre"

// ErrNotAVersion is returned for a tag that is not a version cwm cuts.
var ErrNotAVersion = errors.New("not a version tag")

// Bump is how far a set of commits moves the version.
type Bump int

// The bumps, in increasing order of significance. The zero value is None, so a
// set of commits that says nothing releases nothing.
const (
	// BumpNone is no release: nothing user-facing changed.
	BumpNone Bump = iota
	// BumpPatch is a fix.
	BumpPatch
	// BumpMinor is a new feature.
	BumpMinor
	// BumpMajor is a breaking change.
	BumpMajor
)

// Version is a released version: three numbers, no prerelease, no metadata.
type Version struct {
	Major int
	Minor int
	Patch int
}

// Prerelease is one cut prerelease of a target version.
//
// Number counts within the target, not overall, which is what makes the counter
// restart when a feature commit moves the target from 1.0.2 to 1.1.0.
type Prerelease struct {
	Target Version
	Number int
}

// versionPattern matches a release tag, with or without its leading v.
var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// prereleasePattern matches a cut prerelease tag.
var prereleasePattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)-` + PrereleaseLabel + `(\d+)$`)

// conventionalPattern matches the header of a conventional commit: a type, an
// optional scope, an optional "!" marking a breaking change, then a colon.
var conventionalPattern = regexp.MustCompile(`^([a-zA-Z]+)(\([^)]*\))?(!)?:`)

// ParseVersion reads a release tag such as "v1.2.3".
func ParseVersion(tag string) (Version, error) {
	fields := versionPattern.FindStringSubmatch(strings.TrimSpace(tag))
	if fields == nil {
		return Version{}, fmt.Errorf("%w: %q", ErrNotAVersion, tag)
	}

	// The pattern only matches digits, so these cannot fail.
	major, _ := strconv.Atoi(fields[1])
	minor, _ := strconv.Atoi(fields[2])
	patch, _ := strconv.Atoi(fields[3])

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// ParsePrerelease reads a cut prerelease tag such as "v1.0.1-pre2".
func ParsePrerelease(tag string) (Prerelease, error) {
	fields := prereleasePattern.FindStringSubmatch(strings.TrimSpace(tag))
	if fields == nil {
		return Prerelease{}, fmt.Errorf("%w: %q", ErrNotAVersion, tag)
	}

	major, _ := strconv.Atoi(fields[1])
	minor, _ := strconv.Atoi(fields[2])
	patch, _ := strconv.Atoi(fields[3])
	number, _ := strconv.Atoi(fields[4])

	return Prerelease{
		Target: Version{Major: major, Minor: minor, Patch: patch},
		Number: number,
	}, nil
}

// BumpFor reads one commit message and reports how far it moves the version.
//
// A message that is not a conventional commit moves nothing: cwm would rather
// skip a release than guess what an unlabelled commit did.
func BumpFor(message string) Bump {
	header, body, _ := strings.Cut(strings.TrimSpace(message), "\n")

	fields := conventionalPattern.FindStringSubmatch(header)
	if fields == nil {
		return BumpNone
	}

	// Either spelling of the footer, and the "!" before the colon, mean the
	// same thing.
	if fields[3] == "!" || mentionsBreakingChange(body) {
		return BumpMajor
	}

	switch strings.ToLower(fields[1]) {
	case "feat":
		return BumpMinor
	case "fix", "perf":
		return BumpPatch
	default:
		return BumpNone
	}
}

// BumpForAll returns the largest bump any of the commits asks for.
func BumpForAll(messages []string) Bump {
	largest := BumpNone

	for _, message := range messages {
		if bump := BumpFor(message); bump > largest {
			largest = bump
		}
	}

	return largest
}

// String renders the version without its leading v, the form goreleaser stamps
// into a binary.
func (v Version) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// Tag renders the version as its git tag.
func (v Version) Tag() string {
	return "v" + v.String()
}

// Apply returns the version a bump moves this one to.
//
// A major bump zeroes the minor and patch, and a minor bump zeroes the patch,
// so 1.0.2 with a feature becomes 1.1.0 rather than 1.1.2.
func (v Version) Apply(bump Bump) Version {
	switch bump {
	case BumpMajor:
		return Version{Major: v.Major + 1, Minor: 0, Patch: 0}
	case BumpMinor:
		return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
	case BumpPatch:
		return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
	case BumpNone:
		return v
	default:
		return v
	}
}

// Compare orders two versions, returning -1, 0 or 1.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return sign(v.Major - other.Major)
	}

	if v.Minor != other.Minor {
		return sign(v.Minor - other.Minor)
	}

	return sign(v.Patch - other.Patch)
}

// Tag renders the prerelease as its git tag.
func (p Prerelease) Tag() string {
	return p.Target.Tag() + "-" + PrereleaseLabel + strconv.Itoa(p.Number)
}

// Compare orders two prereleases by target first, then by number.
//
// The number is compared as a number rather than as text, which is what keeps
// pre10 after pre2.
func (p Prerelease) Compare(other Prerelease) int {
	if ordering := p.Target.Compare(other.Target); ordering != 0 {
		return ordering
	}

	return sign(p.Number - other.Number)
}

// mentionsBreakingChange reports whether a commit body carries the footer,
// in either of the two spellings the convention allows.
func mentionsBreakingChange(body string) bool {
	for line := range strings.Lines(body) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "BREAKING CHANGE:") || strings.HasPrefix(trimmed, "BREAKING-CHANGE:") {
			return true
		}
	}

	return false
}

// sign reduces a difference to -1, 0 or 1.
func sign(difference int) int {
	switch {
	case difference < 0:
		return -1
	case difference > 0:
		return 1
	default:
		return 0
	}
}
