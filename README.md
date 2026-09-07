# claude-workspace-manager

`cwm` manages **Claude workspaces**: markdown-first repositories used for working with
Claude on things that aren't code — research notes, journals, topic files, planning
docs, and the `.claude/` configuration that ties them together.

A workspace is an ordinary git repository. What makes it a workspace is its shape:
`CLAUDE.md` at the root, a `.claude/` directory, and content organised as markdown
rather than source.

`cwm` keeps those workspaces in order on disk and on their remotes, and launches
Claude Code sessions inside them.

> **Status:** early. The project structure and a `version` command are in place;
> workspace management itself is not implemented yet.

## Install

Grab a binary from [releases](https://github.com/pdylanross/claude-workspace-manager/releases),
or build from source:

```sh
go install github.com/pdylanross/claude-workspace-manager/cmd/cwm@latest
```

## Usage

```sh
cwm version           # version, commit, build date, Go toolchain, platform
cwm version --short   # just the version string
cwm --help
```

## Development

Requires Go 1.25 and [`just`](https://github.com/casey/just).

```sh
just tools      # install goimports, golangci-lint, goreleaser
just            # fmt, lint, test, build
just run version
just snapshot   # build a local release into dist/ without tagging
```

Individual recipes: `just build`, `just fmt`, `just lint`, `just test`, `just clean`.
Run `just --list` for the full set.

## Layout

| Path             | Contents                                                    |
| ---------------- | ----------------------------------------------------------- |
| `cmd/cwm/`       | Thin entrypoint; holds the link-time build variables.        |
| `internal/cli/`  | Cobra command tree. Not importable outside this module.      |
| `pkg/version/`   | Build information type. Safe for outside consumers.          |

## Releasing

Push a `v*` tag. GitHub Actions runs GoReleaser, which builds linux/darwin ×
amd64/arm64 binaries, stamps the version via ldflags, and publishes a release.

## License

Apache-2.0. See [LICENSE](LICENSE).
