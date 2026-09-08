# Charm libraries

`github.com/charmbracelet/fang` v1.0.0 · `huh` v1.0.0 · `lipgloss` v1.1.0 · `bubbletea` v1.3.6
(indirect, via huh)

Source: `go doc` against the module cache plus the upstream READMEs. Version-exact. One file
because the four are used together and their gotchas are about how they interact.

**All terminal output in this repo goes through these.** No hand-rolled ANSI, no `fmt.Println`
for anything a user reads. See SKILL.md §8.

## Which library for what

| Need | Library | Entry point |
|---|---|---|
| Styled cobra help, usage, errors, `--version` | fang | `fang.Execute(ctx, root, opts...)` |
| A prompt, a confirmation, a form | huh | `huh.NewForm(huh.NewGroup(...))` |
| Colour, alignment, borders on printed output | lipgloss | `lipgloss.NewRenderer(w)` |
| A long-running interactive view | bubbletea | only when huh cannot do it |

Reach for huh before bubbletea. huh *is* a bubbletea program; writing one by hand is only worth
it for something huh has no field for.

## fang

```go
func Execute(ctx context.Context, root *cobra.Command, options ...Option) error
```

Replaces `root.Execute()`. Options: `WithVersion(string)`, `WithCommit(string)`,
`WithoutVersion()`, `WithoutManpage()`, `WithoutCompletions()`, `WithTheme(ColorScheme)`,
`WithColorSchemeFunc(...)`, `WithErrorHandler(func(io.Writer, Styles, error))`,
`WithNotifySignal(...os.Signal)`.

## huh

`huh.NewForm(groups...)` then `.Run()`. Fields: `NewInput`, `NewText`, `NewSelect[T]`,
`NewMultiSelect[T]`, `NewConfirm`, `NewNote`, `NewFilePicker`. Values bind with `.Value(*T)`,
validation with `.Validate(func(T) error)`, and `.Key(k)` makes a value readable afterwards via
`form.GetString(k)` / `GetBool(k)`.

Form methods that matter here:

| Method | Why |
|---|---|
| `WithInput(io.Reader)` | Bind to `cmd.InOrStdin()`. Without it huh reads the real stdin and a test hangs. |
| `WithOutput(io.Writer)` | Bind to `cmd.OutOrStdout()`, for the same reason. |
| `WithAccessible(bool)` | Plain prompt/response instead of a full-screen TUI. Works without a TTY. |
| `WithTheme(*Theme)` | `huh.ThemeCharm()`, `ThemeDracula()`, `ThemeBase()`. |
| `RunWithContext(ctx)` | Cancellable. Prefer it inside a cobra `RunE`. |

## lipgloss

`lipgloss.NewStyle().Bold(true).Foreground(...)`, `JoinHorizontal`, `JoinVertical`, `Width`,
`Border`. `Renderer` carries the colour profile: `lipgloss.NewRenderer(w)` then
`renderer.NewStyle()`.

## Gotchas

1. **The default lipgloss renderer inspects `os.Stdout`, not the writer you print to.** Style
   built from the package-level `lipgloss.NewStyle()` therefore colours according to the
   *process's* stdout. In a test that captures `cmd.OutOrStdout()` into a `bytes.Buffer` this
   emits real ANSI when the test is run from a terminal and none when run in CI — a suite that
   passes on one machine and fails on the other. **Always `lipgloss.NewRenderer(cmd.OutOrStdout())`
   and build styles from that renderer.**
2. **huh reads the real stdin unless told otherwise.** A form without `WithInput` in a test
   blocks forever rather than failing. Bind both `WithInput` and `WithOutput` to the cobra
   command's streams, always.
3. **huh needs a TTY unless it is in accessible mode.** `Run()` against a pipe errors or misbehaves;
   `WithAccessible(true)` degrades to line-oriented prompting. Decide which by asking
   `term.IsTerminal(os.Stdin.Fd())`, never by trying and recovering.
4. **`fang.Execute` handles `--version` itself.** A root command that also sets `cobra.Command.Version`
   has two implementations of the same flag. Pass the version through `fang.WithVersion` and
   leave the field unset.
5. **fang rewrites error text for display**, capitalising the first letter and adding a full
   stop. `fmt.Errorf("show %s: unknown setting", ...)` renders as `Show nope: unknown setting.`
   Keep writing errors the §3.2 way — lowercase, no trailing punctuation — and let fang present
   them. Do not pre-capitalise to match what you see.
6. **fang adds commands.** Manpage and completion commands appear unless switched off. Anything
   that reasons about command names — `skipsUpdateCheck` in `internal/cli/update.go` does —
   must account for them.
7. **Never style machine-readable output.** `cwm config show` is piped into `jq`; a colour code
   in that stream is a bug, not a decoration. Styling belongs on prose, prompts, help and
   errors.
8. **bubbletea's full-screen programs take over the terminal**, which does not compose with a
   cobra command writing to an injected buffer. If one is ever needed, it owns the terminal for
   its duration and returns before anything else prints.
9. **Charm dependencies are numerous and move fast.** `go get` of these four pulled fourteen
   transitive modules. Upgrade them together, and re-read this file when you do.

## Project conventions

- `internal/cli` is the only package that imports any of these. Everything below it returns
  data and errors; nothing below it knows what a terminal is.
- Interactive prompts require a TTY on both stdin and stdout. Every prompt has a flag that
  supplies the same value, so nothing is only reachable interactively (SKILL.md §8).
- Styles come from a renderer bound to `cmd.OutOrStdout()`, never from the package-level
  helpers.
- Commands still write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`. These libraries do
  not change that rule; they change what is written.

## Omitted

`bubbles` (component toolkit), `glamour` (markdown rendering), `log`, `harmonica`, `wish`. All
available, none used yet. Reach for `glamour` if workspace markdown ever needs rendering in the
terminal, and `log` only if structured logging arrives — `log/slog` is the current answer.
