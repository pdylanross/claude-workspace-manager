# `cwm setup`: the first-run flow

Status: design. Depends on the assumptions in `workspaces.md`, chiefly that one cwm
configuration means one forge (§1.7) and that credentials belong to the forge's own CLI (§1.4).

## What it is for

Getting from a freshly installed cwm to one that knows which forge, which space, and where
clones live. It does not clone anything, create anything, or contact the forge beyond proving
that the CLI is authenticated. Discovery is a separate command run afterwards.

Re-running it is normal and safe: it re-asks with the current configuration as the defaults.

## The flow

```
$ cwm setup

  Which forge?
  > GitHub
    GitLab

  ✓ gh is installed and authenticated as pdylanross

  Space          pdylanross
  Workspace root ~/claude-workspaces

  Configure cwm for github:pdylanross? (y/N)

  ✓ written to ~/.config/cwm/config.json
```

1. **Choose the forge.** A select, two options. Defaults to whatever is configured, GitHub
   otherwise.
2. **Check its CLI.** Installed, on `PATH`, and logged in. Nothing after this point is asked
   until this passes, because every remaining default is read from the CLI.
3. **Read the defaults from the CLI.** The account name from `gh api user` or the `glab`
   equivalent; the host from `glab` for a self-hosted instance. Asking someone to type what the
   tool beside it already knows is a bad first impression.
4. **Confirm the space**, pre-filled from step 3, editable — the account is the common case and
   an org or group is a plausible one.
5. **Confirm the workspace root**, pre-filled with the existing setting or
   `~/claude-workspaces`.
6. **Summarise and confirm**, then write. Nothing is written before this point.

## Non-interactive

Every prompt has a flag, per SKILL.md §6. The whole flow above is:

```sh
cwm setup --forge github --space pdylanross --workspace-root ~/claude-workspaces --yes
```

With no TTY and missing flags, cwm fails and names the flag it wanted rather than guessing.
Silently defaulting something a person was about to be asked about is worse than stopping.

Flags supplied interactively skip their question; `--yes` skips the final confirmation.
`cwm setup --forge gitlab --host gitlab.example.com` asks only what is left.

## Failure modes

These are the whole point of the command, and each names the fix:

| Situation | What cwm says |
|---|---|
| `gh` not on `PATH` | how to install it, and that cwm needs it rather than a token |
| `gh` present, not logged in | `gh auth login` |
| Token missing `repo` | `gh auth refresh -h github.com -s repo` |
| Token missing `delete_repo` | a note, not an error: everything except `cwm delete` works |
| Space not visible to the token | the space, and that the CLI is authenticated as somebody else |
| No TTY, no flags | the flag to pass |

The `delete_repo` case is deliberately a warning. It is the one scope a default `gh auth login`
omits, and refusing to finish setup over a capability most runs never use would be the wrong
trade.

## What it writes

The `forge` group from `workspaces.md` §6, plus `workspaceRoot`, through the existing config
store — so normalisation, validation and the schema version all apply unchanged, and
`cwm config set` can edit afterwards anything setup asked about.

Setup is a convenience over the config document, not a second way to store configuration.

## Deliberately not in scope

- **Creating or discovering workspaces.** Setup makes cwm usable; it does not go looking.
- **Storing a credential.** Assumption 1.4. cwm reads the CLI's token per run and keeps none.
- **Choosing between several forges.** Assumption 1.7 is one configuration, one forge. Someone
  who wants both runs `cwm setup` twice under two `CWM_CONFIG_ROOT`s.
