# golang.org/x/mod/semver v0.40.0

Source: `go doc -all golang.org/x/mod/semver` + package docs in the module cache. Version-exact.

Pure string comparison of semantic versions. No types, no state, no errors — every function takes
and returns strings and is total (invalid input yields a defined result, never a panic).

## Hard requirement

**Every version string must have a leading `v`.** `"1.2.3"` is invalid; `"v1.2.3"` is valid. This
is the single most common mistake with this package: `IsValid("1.2.3")` is `false` and
`Compare("1.2.3", "1.2.4")` is `0`, not `-1`. Normalise before calling anything — cwm does this in
`update.canonical`.

Accepted form: `vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]]`. `vMAJOR` and `vMAJOR.MINOR` are
shorthands for `.0` / `.0.0`, which is a deviation from semver.org.

## Signatures

| Function | Returns |
|---|---|
| `IsValid(v string) bool` | Whether `v` parses. |
| `Compare(v, w string) int` | `-1`, `0`, `+1`. An **invalid version sorts below every valid one**, and two invalid versions compare `0`. |
| `Canonical(v string) string` | `v` with shorthands expanded and build metadata dropped. `""` if invalid. |
| `Major(v string) string` | `"v2"`. `""` if invalid. |
| `MajorMinor(v string) string` | `"v2.1"`. `""` if invalid. |
| `Prerelease(v string) string` | `"-beta.1"` **including the leading hyphen**, or `""` when there is none. |
| `Build(v string) string` | `"+meta"` including the leading `+`, or `""`. |
| `Max(v, w string) string` | The larger, by `Compare`. |
| `Sort(list []string)` | Sorts ascending in place. |
| `type ByVersion []string` | `sort.Interface` for the same ordering. |

## Gotchas

1. **No leading `v` means silently wrong answers, not errors.** There is no error return anywhere
   in this package; bad input degrades to `""` or `0`. Guard with `IsValid` first.
2. **`Compare` ranks an invalid version below every valid one.** So a comparison against an
   unparseable current version reports "everything is newer" — exactly the wrong answer for an
   updater. cwm checks `IsValid` before comparing, which is what `update.IsRelease` is for.
3. **`Prerelease` returns the hyphen.** Test with `!= ""`, never against `"beta"`.
4. **Build metadata is ignored for ordering**, per semver.org. `v1.0.0+a` and `v1.0.0+b` compare
   equal.
5. **A prerelease sorts *below* its release**: `v1.2.0-pre < v1.2.0`. This is what makes the
   prerelease channel work without special-casing the comparison.
6. This package does not know about the goreleaser convention of stamping a version *without* the
   `v` into the binary. `version.Info.Version` is `"1.2.3"` and release tags are `"v1.2.3"`; both
   go through `update.canonical` before they meet.

## Project conventions

- Never call this package directly outside `internal/update`. `update.canonical` is the only place
  that adds the `v` and validates, so there is one answer to "is this comparable".
- `update.IsRelease` rejects goreleaser snapshot versions (`0.0.0-SNAPSHOT-<sha>`) before they get
  here: they parse as valid semver prereleases, so `semver` alone would happily "update" a local
  dev build to a real release.

## Omitted

`ByVersion` and `Sort` are unused — cwm picks a single maximum in one pass rather than sorting.
