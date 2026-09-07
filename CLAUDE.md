# claude-workspace-manager

`cwm` is a Go CLI that manages **Claude workspaces** — markdown-first git repos used
for non-code work with Claude (`CLAUDE.md` + `.claude/` + markdown content, e.g.
`topics/`, `journal/`, `research/`, `docs/`). It manages them on disk and on their
remotes, and launches Claude Code sessions inside them.

Only the scaffold and `cwm version` exist so far. Workspace logic is unwritten.

## Load the code-standards skill

**Invoke the `code-standards` skill before touching any code in this repo.** That means
before writing, reviewing, refactoring, or debugging Go; before adding a dependency; and
before running lint, tests, or a release.

It is the authoritative source for:

- the validation procedure every change must pass (build → test → lint, and what else to
  run when config or CLI surface changes),
- Go standards and the strict `golangci-lint` rules that actually bite,
- layout rules for `cmd/` / `internal/` / `pkg/`,
- testing patterns,
- `references/` — dense, version-pinned notes on each non-trivial dependency. **Read the
  relevant reference before writing code against that library.**

Do not re-derive these rules from the code; the skill has them written down, and it
records decisions (like why `export_test.go` wraps a function rather than aliasing a var)
that the code alone doesn't explain.

**Adding a non-trivial dependency requires adding a `references/` file for it in the same
change.** The skill documents what counts as non-trivial and how to write one.

## Facts

- Module: `github.com/pdylanross/claude-workspace-manager`. Binary: `cwm`.
- Go 1.25. Cobra for the CLI. `just` is the task runner — read `justfile` first.
- Releases: push a `v*` tag; GoReleaser handles the rest.
- The linter is strict and `just lint` must report `0 issues`. Never weaken `.golangci.yml`
  or paper over a finding with `//nolint` to make it pass.
