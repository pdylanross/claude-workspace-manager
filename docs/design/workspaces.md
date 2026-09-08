# Workspace management: assumptions and design

Status: design, nothing built. Written before the first line of forge code so that the
assumptions are arguable rather than implied.

*Forge* here means the hosting platform — GitHub, GitLab — as distinct from git itself. The word
is the usual one in this corner of the world, back-formed from SourceForge. It is used in
preference to "SCM" because an SCM is the version control system, which is git; cwm talks to
both, and they need separate names.

## 1. Assumptions

These are choices, not facts. Each one buys a simplification; if one turns out to be wrong,
the section that depends on it is the section to revisit.

1. **The upstream forge is the source of truth for which workspaces exist.** Local clones are a
   cache. There is no local-only mode: a directory that is not backed by a repository on a
   forge is not a workspace, and cwm will not pretend otherwise. This is what lets discovery be
   the only thing that answers "what workspaces do I have", with no local index to keep in
   step.
2. **The user is an admin of the space cwm manages.** cwm may create repositories, set topics,
   and push. This is what makes topic-based discovery viable: cwm is never in the position of
   wanting a label it cannot apply.
3. **A space is personal.** A GitHub account or a GitLab group — one person's area, not a
   shared organisation. Repo counts are in the tens, not the thousands, which bounds the cost
   of anything that has to look at every repository.
4. **Credentials belong to an external CLI, never to cwm.** cwm borrows a token per run and
   stores none. See §5.
5. **Two forges, both of them real: GitHub and GitLab.** GitHub for personal work, GitLab for
   employed work, so both get used and both get tested. Everything else — Azure DevOps, Gitea,
   Forgejo — is explicitly out of scope. Designing around a forge nobody here can exercise
   produces an abstraction that is wrong in ways nobody notices; a contributor who needs one
   can add it against a real seam.
6. **The GitLab instance may be self-hosted.** Work GitLab usually is. Host is configuration,
   not a constant, and `gitlab.com` is a default rather than an assumption.
7. **One cwm configuration means one forge, one space, one workspace root.** Not one per
   machine — one per configuration. Personal GitHub and work GitLab are not meant to be used
   side by side, because they are not on the same machine; the personal one lives on a personal
   computer and the work one on a work laptop.

   Somebody who does want both on one machine runs two configurations, which is what
   `CWM_CONFIG_ROOT` and `CWM_CACHE_ROOT` already exist for, each pointing at its own workspace
   root. **Two configurations sharing a workspace root is unsupported and its behaviour is
   undefined.** cwm neither prevents nor detects it.

   This is the assumption that pays for the most simplification downstream. Discovery has one
   space to search, configuration has one forge to describe, and the clone layout in §7 becomes
   provably safe rather than merely usually safe.

## 2. What makes a repository a workspace

**The topic.** A repository carrying the `cwm-workspace` topic is a workspace; one without it is
an unrelated repository that cwm does not touch, whatever files it happens to contain. Both
supported forges have topics and assumption 2 says cwm can always set them, so there is no case
to handle where the marker cannot be applied.

`.cwm.json` at the repository root is a different thing: the workspace's own configuration, and
the only marker that exists once a workspace is a directory on disk rather than a URL. It
answers "how is this workspace set up" and "am I standing in one", not "does this exist".

Splitting the two jobs is what makes discovery one API call. The topic is on the listing cwm
already has; the file is behind a round trip per repository. Nothing in discovery needs to read
it.

A workspace whose `.cwm.json` is missing is not an error — it is a workspace running on
defaults, exactly as a config root with no `config.json` is. cwm writes the file when it first
needs to record something.

### The schema

Same treatment as cwm's own configuration, and a separate schema with its own version. JSON,
`schemaVersion` from the first release, addressable by dotted path, and the struct rules from
the code-standards skill §3.4 apply unchanged: no slices, arrays, maps or embedded structs, and
enums as named string types.

It will be nearly empty to begin with and is expected to carry a lot eventually, which is the
argument for taking the version and the shape seriously now. It is also the one document cwm
cannot migrate by rewriting: copies live in repositories cwm does not control, some of them
cloned by people running older builds, so the version is the only way an old cwm can recognise
a document it should not touch.

## 3. Discovery

**The topic is the whole of it.** Discovery is one filtered listing, and what comes back is the
answer — no confirmation pass, no file fetches, nothing to reconcile.

The alternative, listing every repository and fetching `.cwm.json` from each, is correct and too
slow to run often: a hundred repositories is a hundred round trips, almost all of them 404s.

Verified, September 2026:

| Forge | Repo topics | Server-side filter |
|---|---|---|
| GitHub | yes, returned inline in the repo listing | `GET /search/repositories?q=topic:cwm-workspace user:X` |
| GitLab | yes, `topics` (replaced `tag_list`) | `GET /projects?topic=cwm-workspace` |

Both supported forges can do this, so discovery is one API call on either and there is no
fallback to build. A forge without repository topics — Azure DevOps has none, its "tags" being
git tags and pull request labels — would implement `Discover` by listing everything and
fetching `.cwm.json` from each. Assumption 3 is what would make that acceptable. It is not
written until somebody needs it.

Two consequences:

- **`Discover` is the interface boundary, not `ListRepositories` + `GetFile`.** Those two
  methods would force every forge into the slow shape. The two supported forges happen to
  agree on how to be fast, which makes it tempting to skip the seam; the seam is cheap and it
  is exactly where a forge that cannot do this would plug in.
- **Discovery never reads `.cwm.json`.** It answers "what is there", not "what is in it". The
  file is read when a workspace is opened or cloned, where a round trip is already being paid
  for.

## 4. Creating and deleting

Both ends of a workspace's life go through the forge, because of assumption 1.

**`cwm new` creates the repository, marks it, and only then does the rest:**

1. Create the repository on the forge.
2. **Set the topic.**
3. Clone it.
4. Write `.cwm.json` and whatever scaffold a workspace starts with.
5. Commit and push.

The topic goes on as early as it can, and that is the opposite of what an earlier draft of this
document said. It was written when the file was the marker, and under that rule the topic had to
be set last so it could only ever lag the file. Making the topic authoritative inverts the
argument entirely: the topic is now the thing that makes the repository *exist* as far as cwm is
concerned, so anything that happens before it is set happens to a repository cwm can never find
again.

That is what makes a partial failure recoverable rather than an orphan. A `cwm new` that dies
after step 2 leaves a repository that discovery finds, so the next run picks it up, clones it,
and finishes the scaffold. A `cwm new` that dies between steps 1 and 2 leaves a repository cwm
is blind to — which is a narrow window, but it is the reason the window should be narrow.

Rolling back — deleting a repository because a clone failed — is a lot of destruction for a
transient error, and is not what cwm does. The repository exists, so by assumption 1 the
workspace exists. cwm should say so rather than apologise.

**`cwm delete` is deliberately not designed hard.** Removing the clone and telling the user to
delete the repository themselves is an acceptable first implementation: it is the safe half of
the operation, and the unsafe half is one click away in a UI that already asks for
confirmation properly.

Deleting upstream too is the nicer version, and if it is built it wants the workspace name typed
rather than a `y/N`, both the URL and the local path shown, and a refusal when the clone holds
commits that were never pushed. Everything else being destroyed exists in two places; unpushed
work exists in one.

This is also the one operation with a credential problem worth knowing about in advance. A
default `gh auth login` grants `gist, read:org, repo` — enough to create a repository and set
its topics, so `cwm new` works out of the box, and *not* enough to delete one. Whichever
implementation is chosen, cwm should check for `delete_repo` before it asks for confirmation
rather than after, and put `gh auth refresh -h github.com -s delete_repo` in the error. Nagging
the user to go and delete it by hand sidesteps this entirely, which is a point in favour of the
simpler version.

## 5. Credentials

cwm stores no credentials and implements no OAuth flow. It asks the forge's own CLI for a
token, per run, and never writes it anywhere.

| Forge | CLI | Token |
|---|---|---|
| GitHub | `gh` | `gh auth token` |
| GitLab | `glab` | `glab auth status --show-token` |

`glab` also holds the host it is logged in to, which is the natural place to learn where a
self-hosted instance lives rather than asking the user twice.

This is worth the shelling out. Those tools already solve login, refresh, revocation, keyring
storage and enterprise SSO, and a user who has one installed has already authenticated with
it. The alternative — cwm running its own device flow and owning a token file — is a
credential store to get wrong for no benefit.

Consequences to design for:

- **A missing or unauthenticated CLI is a first-class error**, with the command to run in the
  message. It is the single most likely thing to go wrong on first use.
- **Scopes are the user's problem but cwm's job to diagnose.** A default `gh auth login` covers
  everything except deletion; see §4.
- **The token never reaches a log, an error message, or the config document.**
- Shelling out per run, not per request: one exec, then a typed client.

## 6. The forge interface

Sketch, not a signature. The shape that matters is that discovery, not listing, is what a
forge implements.

```go
// Forge is one hosting provider, authenticated as the user by its own CLI.
type Forge interface {
    Name() string

    // Available reports whether this forge's CLI is installed and logged in,
    // and says what to run when it is not.
    Available(ctx context.Context) error

    // Discover returns the workspaces in the space, however this forge can
    // find them most cheaply. It does not read .cwm.json.
    Discover(ctx context.Context, space Space) ([]Candidate, error)

    // Describe reads one candidate's .cwm.json, confirming it is a workspace.
    Describe(ctx context.Context, candidate Candidate) (Workspace, error)

    // Create makes a new workspace repository and marks it.
    Create(ctx context.Context, space Space, spec Spec) (Workspace, error)
}
```

`Space` is a forge plus a namespace path: a GitHub account or org, a GitLab group. Still not a
bare `owner string`, but it no longer has to stretch around a two-level organisation/project
hierarchy.

Assumption 7 means cwm's own configuration holds exactly one of these, which suggests a `forge`
group alongside `update`:

```json
"forge": { "kind": "github", "host": "", "space": "pdylanross" }
```

`kind` is an enum in the §3.4 sense — `github` or `gitlab` — and `host` is empty for the public
instance, which is how a self-hosted GitLab gets named without a second setting to say whether
one is in use.

For the GitLab implementation, the maintained Go client is `gitlab.com/gitlab-org/api/client-go`
(the former `xanzy/go-gitlab`). It will need a `references/` file when it is added, as
go-github did.

## 7. Where clones live

Flat under `workspaceRoot`: one directory per workspace, named for the repository. Nothing
nested, nothing computed from the forge or the namespace.

Assumption 7 is what makes this safe rather than merely tidy. One configuration searches one
space, and within a single namespace a repository name is unique by construction — a forge
cannot hold two repositories at `owner/notes`. So a name collision is not something flat layout
risks; it is something that cannot arise in a supported configuration.

There are two ways to leave that guarantee, and both are worth naming because neither is
obvious:

- **Sharing one workspace root between two configurations**, which assumption 7 says is
  unsupported. Personal `notes` and work `notes` would collide in exactly the way this design
  otherwise rules out.
- **Recursing into GitLab subgroups.** `group/a/notes` and `group/b/notes` are distinct
  projects with the same name, so a space that spans subgroups can collide with itself. The
  API's `include_subgroups` parameter defaults to `false`, and cwm should leave it there. If
  recursive spaces are ever wanted, this is the thing that has to be solved first, not an
  afterthought.

cwm should still refuse to clone over a directory that is already occupied. That is not a
design for collisions — it is the guard that turns an unsupported configuration into an error
message instead of a silently clobbered workspace.

## 8. Open questions

- **Whether the addressing layer gets generalised.** `.cwm.json` wants what `pkg/config` already
  has — `Settings`, `Get`, `Set`, `Render`, `CheckShape`, `Enum`, the `cwm:"path"` tag — and all
  of it is currently written against the `Config` type specifically. Two documents with the same
  treatment means either duplicating that machinery or parameterising it over the document type.
  Parameterising is clearly right and is a bounded refactor, but it is a refactor, and it is
  better done before the second document exists than after.
