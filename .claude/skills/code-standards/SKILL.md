---
name: code-standards
description: Go code standards, validation/testing procedures, and dense library references for claude-workspace-manager (cwm). Load before writing, reviewing, or modifying ANY Go code in this repo, before adding a dependency, and before running lint/test/release. Covers the strict golangci-lint ruleset, project layout rules, testing patterns, and per-library references in references/.
---

# cwm code standards

Authoritative rules for writing Go in this repo. This file is normative; `references/` holds
per-library detail. All content optimized for LLM consumption — density over readability.

## 0. Fast facts

| | |
|---|---|
| Module | `github.com/pdylanross/claude-workspace-manager` |
| Binary | `cwm` |
| Go | 1.25 |
| Task runner | `just` (read `justfile`; never hand-roll equivalents) |
| Linter | `golangci-lint` v2, maratori golden config, `.golangci.yml` |
| Release | `goreleaser` on `v*` tag; tags cut by CI — see §7 |
| CLI framework | cobra → `references/cobra.md` |

## 1. Validation procedure

Run in this order. **Every step must pass before work is reported complete.** Do not report
"done" on a subset.

```sh
go build ./...            # 1. compiles
go test ./... -race       # 2. tests + race detector    (just test)
just lint                 # 3. MUST be 0 issues         (golangci-lint run --fix)
```

Config-level validation, when the corresponding file was touched:

```sh
golangci-lint config verify   # after editing .golangci.yml
goreleaser check              # after editing .goreleaser.yml
```

Behavioural validation, when CLI surface changed:

```sh
just run <cmd>                # exercise the real command
just snapshot                 # full release build, all 4 targets, into dist/
./dist/cwm_linux_amd64_v1/cwm version   # PROVES ldflags reached main.*
```

Rules:
- `just lint` returning anything other than `0 issues.` is a failure, not a warning.
- Never add `//nolint` to make lint pass without an explicit reason string. `nolintlint` requires
  both a specific linter name and an explanation: `//nolint:gosec // reason here`.
- Never weaken `.golangci.yml` to make code pass. Fix the code. Changing the linter config is a
  deliberate, called-out decision, not a workaround.
- `just snapshot` is the ONLY check that catches a broken ldflag path in `.goreleaser.yml`; that
  failure is silent until a real release. Run it whenever `main.go` vars or `.goreleaser.yml` change.

## 2. Layout rules

| Path | Rule |
|---|---|
| `cmd/cwm/` | Entrypoint ONLY. Declares `version`/`commit`/`date` ldflag targets, builds `version.Info`, calls `cli.Execute`. Names must match `-X main.*` in `.goreleaser.yml`. Nothing else goes here. |
| `cmd/release/` | Release tooling, not part of cwm. `.goreleaser.yml` builds `./cmd/cwm` alone, so nothing here ships in the installed binary. Shells out to git; the rules it applies live in `internal/release`. |
| `internal/` | cwm-only library code. **Default home for new subsystems.** |
| `pkg/` | Only code an outside consumer could import. Put something here only if willing to keep its API stable. |

- `internal/cli/` owns the cobra tree. Commands are `newXxxCmd(deps) *cobra.Command` constructors,
  registered in `newRootCmd`. No package-level command vars.
- Dependencies flow down as parameters. `cmd/cwm` → `internal/cli` → `internal/*`. Never upward.

## 3. Go standards

### 3.1 Hard constraints from the linter

These are the rules that actually bite. Write to them the first time.

| Rule | Linter | Detail |
|---|---|---|
| No package-level `var` outside `cmd/` | `gochecknoglobals` | Construct values, pass them down. `version.Info` is a parameter, not a global. Excluded only under `^cmd/` (ldflag targets need it). |
| No `init()` outside `cmd/` | `gochecknoinits` | Wire things explicitly in constructors. |
| Tests live in `package foo_test` | `testpackage` | Skip-regexp allows `export_test.go` and `internal_test.go` in `package foo`. |
| Comments end in a period | `godot` | Every doc comment. Exception: lines containing `TODO`. |
| Stdlib doc refs are bracketed | `godoclint` | `[os.Args]`, `[io.Writer]` — not bare `os.Args`. Also `no-unused-link`. |
| No named returns | `nonamedreturns` | |
| No magic numbers | `mnd` | Named constants. Some funcs exempted (see config `ignored-functions`). |
| Max line length 120 | `golines` (formatter) | Auto-fixed by `just fmt`/`--fix`. |
| Errors wrapped for `errors.Is/As` | `errorlint` | `fmt.Errorf("...: %w", err)`. |
| Sentinel errors named `ErrX`, types `XError` | `errname` | |
| Type assertions checked | `errcheck` (`check-type-assertions`) | `v, ok := x.(T)`, never bare `x.(T)`. |
| `switch`/`map` over enums exhaustive | `exhaustive` | |
| Exported methods before unexported | `funcorder` | Constructors first, then exported, then unexported. |
| `math/rand/v2`, not `math/rand` | `depguard` | Also: `log/slog` not `log` (outside `main.go`); `google.golang.org/protobuf` not `github.com/golang/protobuf`; `github.com/google/uuid` not satori. |
| No `sync.Mutex` as an embedded field | `embeddedstructfieldcheck` | Name the field. |
| No `context.Context` in structs | (convention) | Pass as first arg. |
| `slog` calls take a context in scope | `sloglint` | `no-global: all`, `context: scope`. No global loggers. |
| Errors from other packages must be wrapped | `wrapcheck` | `fmt.Errorf("doing x: %w", err)` at every boundary. Excluded in `_test.go`. |
| Struct literals must set every field | `exhaustruct_v5` | Applies to **our own** structs. Third-party structs with many optional fields (`cobra.Command`, `http.*`, `url.URL`, `exec.Cmd`, `tls.Config`) are filtered by a `text:` exclusion rule; extend that regex rather than adding per-literal directives. Excluded in `_test.go`. |
| Struct tags aligned | `tagalign` | Auto-fixed by `--fix`. |
| Declaration order/count | `decorder` | |
| Interface method params named | `inamedparam` | `skip-single-param: true`. |
| Interfaces stay small | `interfacebloat` | |
| Preallocate slices with known length | `prealloc` | |
| Stricter formatting than gofmt | `gofumpt` (formatter) | Auto-fixed by `just fmt`. |

Complexity ceilings: `funlen` 100 lines / 50 statements, `gocyclo`/`cyclop` 30, `gocognit` 20,
package-average cyclop 10. Exceeding one means the function should be split, not annotated.

`_test.go` files are exempt from: `bodyclose`, `dupl`, `errcheck`, `exhaustruct_v5`, `funlen`,
`goconst`, `gocognit`, `gosec`, `noctx`, `unparam`, `wrapcheck`. They are **not** exempt from
`gochecknoglobals`.

**`exhaustruct_v5` note.** v5 dropped v3's `include`/`exclude` type filters — its only settings are
`allow-empty-returns`, `allow-empty-declarations`, `allow-empty` (verified by probing the config
schema). Third-party struct noise is therefore filtered by a `text:` rule under
`linters.exclusions.rules`, not by linter settings. When a new dependency's structs start generating
noise, extend that regex. The per-literal `//exhaustruct:ignore` directive also works if a one-off
escape is needed. Plain `exhaustruct` (v3) is deprecated since golangci-lint v2.13.0 — do not
re-enable it.

### 3.2 Conventions

- **Errors**: wrap with context at each boundary (`fmt.Errorf("read workspace %s: %w", path, err)`).
  Lowercase, no trailing punctuation. Sentinel errors as `var ErrNotFound = errors.New(...)` at
  package level are fine — `gochecknoglobals` whitelists `Err`-prefixed error vars (verified against
  this config). `reassign` then forbids reassigning them, which is the desired behaviour.
- **Output**: commands write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never `fmt.Println` or
  `os.Stdout` directly. This is what makes commands testable.
- **Context**: accept `context.Context` as the first parameter on anything doing I/O. Inside a cobra
  command use `cmd.Context()`.
- **Interfaces**: define at the consumer, accept interfaces, return concrete types. Keep them small.
- **Doc comments**: every exported symbol. Start with the symbol name, end with a period.

### 3.3 Testing

- Table-driven, subtests via `t.Run`.
- `t.Parallel()` on both the parent test and inside each subtest closure.
- `package foo_test`. To reach unexported identifiers, add `export_test.go` in `package foo`
  exporting a **function** wrapper — a `var` alias trips `gochecknoglobals`, which is not excluded
  for test files:
  ```go
  // export_test.go, package foo
  func NewRootCmd(info version.Info) *cobra.Command { return newRootCmd(info) }
  ```
- Cobra commands are tested by building the tree, `SetOut`/`SetErr` to a `bytes.Buffer`, `SetArgs`,
  then `Execute()`. See `internal/cli/root_test.go`.
- Assert on behaviour and output, not on internal call order. Prefer stdlib `testing`; no assertion
  library is a dependency yet (`testifylint` is configured should `testify` ever be added).
- Test error paths, not just happy paths. `-race` is mandatory.

### 3.4 Config structs

`pkg/config.Config` and every struct nested in it are addressed by dotted path from the CLI
(`cwm config set github.repoPrefix=x`, `cwm config show github`). That addressing is what
constrains their shape.

**Never put a slice, array, or map in a config struct.** There is no stable key for
`things[2]` or `things["k"]`: `config set` could not name it, `config show` could not print a
single element of it, and any ordering the user relied on would be an accident of how the file
was last written. This is not a style preference — it breaks the CLI surface.

| Allowed at a leaf | Never |
|---|---|
| `string`, `bool`, `int`/`int64`, `uint`/`uint64`, `float64` | slice, array, map |
| a named struct, to group settings one level deeper | pointer, interface, channel, func, `any` |
| | an embedded struct (see below) |

Rules:

- **Every field needs a `json` tag.** The tag, not the Go field name, is the on-disk name and
  the name a user types. It is the compatibility promise; renaming one is a breaking change.
  `musttag` enforces the tag's presence, not its stability.
- **Group with nested structs**, not with prefixes in field names: `Github struct { Enabled
  bool }` gives `github.enabled` for free.
- **Never embed a struct.** `encoding/json` promotes an embedded struct's fields to the level
  above, while a path addresses them one level down — the file and the CLI would disagree
  about what the setting is called. Give the field a name.
- **A config struct must stay copyable by assignment.** `cfg2 := cfg` is relied on as a deep
  copy so a failed `config set` writes nothing. Banning reference types is what makes that true.
- **Want a list?** Model it as a nested struct with named fields, or keep it out of the config
  document entirely — its own file under the config root, addressed by its own commands.
- `pkg/config.CheckShape` walks the type and rejects anything unsupported;
  `TestConfigShapeIsAddressable` runs it. A field of a banned kind fails the test suite rather
  than failing at runtime in front of a user.

#### The `cwm` struct tag

Alongside `json`, fields carry cwm's own metadata in a `cwm:"..."` tag (comma-separated):

| Option | Meaning |
|---|---|
| `path` | A string holding a filesystem path. `Config.Normalize` expands a leading `~` and cleans it; `Config.Validate` requires it to be absolute. **Every path setting must have it** — a shell may not expand the tilde (bash does after `=`, zsh does not), so cwm cannot assume it received an expanded path. |
| `internal` | A field cwm maintains for itself. Written to the document, excluded from `Settings()`, and rejected by `Get`/`Set` with `ErrNotASetting`. |

**Enum settings** are a named string type implementing `config.Enum` (`Values() []string`), not a
bare `string` with a comment. `Config.Set` rejects a value outside the set before it lands and
lists the alternatives in the error; `Config.Validate` catches the same thing in a hand-edited
document. Declare the constants in the order a user should see them, default first — `Values()`
output is what the error message prints. See `config.Mode` and `config.Channel`.

Path handling is tag-driven rather than hand-written per field, because forgetting to expand a
new path setting is a user-visible bug. Default-filling (`withDefaults`) is the opposite: it is
written out per setting, because "blank means unset" is a per-setting judgement — a false bool
is a choice, and an empty string may be a deliberate "no prefix".

#### Schema version

`config.SchemaVersion` describes the **document format** and is unrelated to the cwm binary's
version. It moves rarely.

- **Adding a setting does not bump it.** An older document lacks the field and gets the default;
  that is the normal, expected path and needs no migration.
- **Bump only for a change an older cwm would read _wrongly_ rather than not at all**: a renamed
  or repurposed setting, or a value whose meaning changes.
- A document from a *newer* schema is refused on load (`ErrUnsupportedSchema`), and the error
  says to upgrade cwm — never to reset, which would throw away settings to fix the wrong problem.

## 4. Library references

`references/` holds dense, version-pinned notes for each non-trivial dependency. **Read the relevant
reference before writing code against that library** — they exist because the LLM-visible API surface
is larger than what is recallable accurately.

| Library | Version | File | Scope |
|---|---|---|---|
| `github.com/spf13/cobra` | v1.10.2 | `references/cobra.md` | CLI framework: commands, flags, args validation, lifecycle hooks, completions, testing |
| `golang.org/x/mod/semver` | v0.40.0 | `references/semver.md` | Semantic version comparison for the updater |
| `github.com/google/go-github/v76` | v76.0.0 | `references/go-github.md` | GitHub REST API: releases today, more as cwm grows |

Not yet referenced (add on first non-trivial use): git operations, structured logging, TUI.

## 5. Adding a new reference

**Trigger: add a reference whenever a non-trivial library is added to `go.mod`.** Non-trivial means
anything with its own concepts, lifecycle, or API surface wider than a handful of functions — a CLI
framework, a git library, an HTTP/API client, a config loader, a TUI toolkit, an ORM. Skip it for
single-purpose utilities whose entire API is one or two obvious calls.

Do this in the same change that adds the dependency, not later.

### Procedure

1. **Source the docs from the module cache, not the web.** The cache is version-exact and complete:
   ```sh
   C=$(go env GOMODCACHE)/<module>@<version>
   find "$C" -iname '*.md' -not -path '*/vendor/*'   # README, site/, docs/
   ```
   Many projects ship their full documentation site under `site/content/` or `docs/`. Read all of it.
2. **Get the authoritative API surface** — docs drift, signatures do not:
   ```sh
   go doc -all <module> | head -400
   go doc -all <module> | grep -E '^func |^type '
   ```
3. **Fall back to WebFetch** only when the module ships no markdown (`pkg.go.dev/<module>`, or the
   upstream repo docs). Note in the file that it was web-sourced.
4. **Write `references/<library>.md`.** Rules:
   - **This is for an LLM, not a human. Human readability does not matter.** No prose warm-up, no
     narrative, no motivational framing, no "as you can see". Tables, signatures, terse bullets.
   - Pin the version in a header line. Note how it was sourced.
   - Lead with the decision-relevant material: the shapes you actually construct, the exact
     signatures, the semantics that are non-obvious.
   - Include a **Gotchas** section — behaviour that is surprising, silently wrong, or a common
     mistake. This is the highest-value part of the file.
   - Include a **Project conventions** section — how *this* repo uses the library, and which of the
     library's idioms are forbidden here (usually the global-var/`init()` patterns most Go library
     docs demonstrate, which `gochecknoglobals`/`gochecknoinits` reject).
   - Include a **Testing** section if the library has a testing story.
   - Omit anything the project will never use, but say explicitly that it was omitted, so a later
     reader knows it exists.
   - Prefer a compact signature table over paragraphs of description.
5. **Register it** in the §4 table above, with version and scope.
6. **Update on upgrade.** When a dependency's version changes in `go.mod`, re-check its reference: at
   minimum re-run step 2 and diff the API surface. Update the pinned version line either way.

## 6. Pull requests

Derived from PR #1 (`feat: scaffold cwm project structure and version command`) and PR #2
(`feat: add the config subsystem and cwm config commands`). Read the most recent merged PR before
writing a new one; this section is the pattern, not a replacement for looking.

### When a pull request is not needed

**Documentation goes straight to main.** A design document, or a fix to this skill, gains nothing
from a branch and a PR when the author is the only reviewer, and the release pipeline already
ignores it.

"Documentation" means both halves of that, and they have to agree:

- **Only markdown changed** — `docs/`, `*.md`, `.claude/skills/**`. One Go file in the diff and
  it is a PR.
- **The commit is typed `docs:`** — which is what makes it release-neutral. Per §7, `docs:` earns
  no bump, so nothing is tagged and no approval is queued.

The failure mode when the two disagree is quiet in both directions, which is the reason to state
it:

- `feat:` on a markdown-only commit **cuts a release** for a documentation change.
- `docs:` on a commit carrying code ships that code **with no version bump** — invisible to the
  pipeline, and to everyone whose cwm is deciding whether to update.

CI still runs on every push to main, so lint, tests and build are not skipped. Only the review is.

### Mechanics

```sh
gh pr create --base main --head <branch> --title "<title>" --body-file <path>
```

- **Always `--body-file`**, never an inline `--body`. Bodies contain backticks, quotes and `$`;
  passing one through the shell mangles it. Write the file to the scratchpad, not the repo.
- Branch names are `<type>/<topic>`: `feat/config-paths`.
- Title is the same conventional-commit form as a commit subject — `feat: `, `fix: `, `refactor: `,
  lowercase after the prefix, no trailing period. It names the whole branch's theme, not the last
  commit's.

### Body shape

The skeleton, in order:

1. **One opening paragraph** saying what the change is, and — this is the load-bearing half — what
   it deliberately is *not*. PR #1: "This commit is structure only -- no workspace logic yet."
   PR #2: "No workspace logic yet -- this is the layer everything else will read its settings from."
2. **A `Layout:` block** whenever directories are added, two-space indented, `path/` then what lives
   there. Same for a `Tooling:` block listing what was wired up, or a plain block of command lines
   for a new CLI surface.
3. **Prose explaining the decisions**, one topic per paragraph. Not a changelog, not a bullet list
   of files touched — the diff already says what changed. Say why the non-obvious call was made and
   what the alternative would have cost.
4. **The one gotcha, in depth.** Every PR so far has one thing that looks wrong until explained:
   PR #1 why `exhaustruct_v5` is filtered by a `text:` rule; PR #2 why embedded structs are banned.
   Give it a full paragraph. A reviewer who has to reverse-engineer it will ask in a comment
   instead, which is slower for everyone.
5. **A `Verified` section** listing what was actually run — the §1 procedure plus anything
   behavioural, with real numbers. Never claim a check that was not run.
6. **A "Note for reviewers" line** for known gaps: what is untested, illustrative, or deferred, and
   why. Volunteering the weak spot is cheaper than having it found.
7. **The harness-provided attribution footer last**, when Claude Code authored the PR.

### Formatting

- **ASCII `--` for em dashes**, not `—`. Both existing PRs do this.
- **Inline code sparingly.** PR #1 uses backticks on two lines total. Reserve them for identifiers
  and commands where the monospace actually disambiguates; prose about a package does not need
  them on every mention.
- **No markdown headings in a single-subsystem PR** — PR #1 has none, and prose blocks carry it.
  Add `###` headings only when the PR spans several packages and the reader needs to navigate;
  PR #2 earns them at six commits across three packages. Pick one and hold it for the whole body.
- No emoji beyond the attribution footer. No collapsed `<details>` blocks. No screenshots for a CLI
  — paste the actual terminal output instead.

### Commit messages inside the PR

The PR body is written *from* the commit bodies, condensed — so write each commit message as if it
were going to be read by a reviewer, because it is. Same rules: subject in conventional-commit
form, body in prose explaining why, `--` for em dashes, and the harness attribution trailers last.

## 7. Release pipeline

Versions are derived from commit messages, not chosen by hand. `internal/release` holds the rules
(pure, tested against the worked example in `plan_test.go`); `cmd/release` reads git and prints
`key=value` lines a workflow appends to `$GITHUB_OUTPUT`.

### What a commit is worth

Conventional commits, largest wins across everything since the last **real** release:

| Commit | Bump |
|---|---|
| `feat:` | minor |
| `fix:`, `perf:` | patch |
| any type with `!` before the colon, or a `BREAKING CHANGE:` / `BREAKING-CHANGE:` footer | major |
| everything else (`docs:`, `chore:`, `test:`, `refactor:`, `ci:`, unlabelled) | none |

**No bump means no release.** A docs-only push moves main and cuts nothing. This is why commit
subjects matter: an unlabelled commit is invisible to the pipeline.

### The cycle

Every push to main, after lint/test/build pass, cuts a prerelease `vX.Y.Z-preN`. The gated
`promote` job turns the newest one into `vX.Y.Z` once a reviewer approves it. A push that cuts no
prerelease queues no approval: the job's `if` is evaluated before the environment gate.

**Concurrency belongs on the release jobs, never on the workflow.** A job parked on an approval
counts as in progress, so a workflow-level group would hold up lint and test for every later push
until somebody dealt with the approval. The two release jobs want opposite settings: cutting is
serialised and never cancelled (`cancel-in-progress: false`) because a half-written release is worse
than a slow queue, while promotion *is* cancelled by the next one (`cancel-in-progress: true`) so a
newer prerelease withdraws the older one's approval request. Approving is then always approving the
newest thing on main, and stale requests never pile up.

**The prerelease counter is scoped to the target version, not to the timeline.** When a `feat:`
arrives after `1.0.2-pre1` has been cut, the target moves to `1.1.0`, whose counter has never been
used, so the next cut is `1.1.0-pre1` and `1.0.2-pre1` is simply left behind. Abandoned tags are
normal and are never cleaned up.

```
1.0.0 ─ fix ─> 1.0.1-pre1 ─ fix ─> 1.0.1-pre2 ─ approve ─> 1.0.1
      ─ fix ─> 1.0.2-pre1 ─ feat ─> 1.1.0-pre1 ─ fix ─> 1.1.0-pre2 ─ feat ─> 1.1.0-pre3 ─ approve ─> 1.1.0
                    ^ abandoned
```

### Rules

- **The gate is the `release` environment's required reviewer.** Approving the pending deployment
  is what promotes. Two settings on that environment are load-bearing and easy to get wrong:
  `prevent_self_review` must stay **false**, or the person who pushed can never approve their own
  release; and the deployment branch policy must **name `main` explicitly** rather than being set to
  "protected branches", because `main` carries no branch protection and the deployment would be
  rejected before an approval was ever requested. Required reviewers need a paid plan on a *private*
  repository — the API answers 422 — which is one reason this repository is public.
- **Promotion tags the prerelease's commit, not HEAD.** main moves while approval waits; releasing
  HEAD would ship code that was never in the prerelease. The workflow checks the tag out detached
  before goreleaser runs.
- **Always pass `GORELEASER_CURRENT_TAG`.** A promotion leaves two tags on one commit, and asked to
  work out which it is on, goreleaser sorts them with git's version sort — which knows nothing about
  semver prereleases and ranks `v0.1.0-pre1` *above* `v0.1.0`. It then re-releases the prerelease it
  was promoting, fails on `422 already_exists` uploading assets that are already there, and leaves a
  release tag with no release behind it. `git describe --tags --abbrev=0` answers correctly on the
  same commit, so this is not visible from a quick check; naming the tag outright is the only
  reliable fix. (`git config versionsort.suffix -pre` would fix the sort, but it depends on how
  goreleaser looks the tag up, which is not ours to rely on.)
- **CI runs goreleaser itself** rather than relying on `release.yml`. A tag pushed with
  `GITHUB_TOKEN` does not trigger another workflow — GitHub's loop protection — so a tag-triggered
  release would silently never fire. `release.yml` remains for tags pushed by hand.
- **Prerelease tags are `-preN`, not `-pre.N`.** N is compared as a number everywhere cwm compares
  it, but *semver* compares `pre1` and `pre10` as text, so `pre10` sorts before `pre2`. Nothing in
  the pipeline depends on semver ordering; `internal/update` does. Changing the label to `pre.N`
  fixes it and only `PrereleaseLabel` and one regexp would move.
- Bumping is literal: a breaking change below 1.0.0 still cuts a major. There is no "0.x is
  special" rule.
