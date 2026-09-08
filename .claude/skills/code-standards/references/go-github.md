# github.com/google/go-github/v76 v76.0.0

Source: `README.md` in the module cache + `go doc` on the used types. Version-exact.

REST (v3) client. Import path carries the major version: `github.com/google/go-github/v76/github`.
A major bump is a **new import path** — upgrading means rewriting every import, and two majors can
coexist in one build. GraphQL is a different library (`shurcooL/githubv4`), not used here.

## Construction

```go
api := github.NewClient(httpClient)   // nil ⇒ http.DefaultClient, no timeout
api.UserAgent = "cwm/1.2.3 (linux/amd64)"
api.BaseURL, _ = url.Parse(server.URL + "/")   // trailing slash REQUIRED
```

| Field | Notes |
|---|---|
| `BaseURL *url.URL` | Defaults to `https://api.github.com/`. **Must end in `/`** or request paths resolve wrongly. |
| `UploadURL *url.URL` | Only for uploads. Unused here. |
| `UserAgent string` | Sent on every request. |
| `DisableRateLimitCheck bool` | Stops client-side rate-limit tracking. |
| `Repositories`, `Actions`, `Issues`, … | Services; the API is divided across ~50 of them. |

`WithAuthToken(token)` returns a client that sends `Authorization: Bearer`. `WithEnterpriseURLs`
rewrites the base URL and **appends `api/v3/`** unless the URL already contains it — surprising for a
test server, so set `BaseURL` directly instead.

## Signatures used

| Call | Returns |
|---|---|
| `Repositories.ListReleases(ctx, owner, repo, *ListOptions)` | `[]*RepositoryRelease, *Response, error` |
| `Repositories.GetLatestRelease(ctx, owner, repo)` | `*RepositoryRelease, *Response, error` — **excludes prereleases and drafts**, so it cannot serve a prerelease channel. |
| `Repositories.DownloadReleaseAsset(ctx, owner, repo, id int64, followRedirects *http.Client)` | `rc io.ReadCloser, redirectURL string, err error` |

`ListOptions` is `{Page, PerPage int}`. Page 0 means the first page.

## Gotchas

1. **Every field is a pointer.** `release.TagName` is `*string`. Dereferencing a nil one panics on a
   field the API happened to omit. **Always use the generated `GetXxx()` accessors** — they nil-check
   and return the zero value. This is the single most common way to crash with this library.
2. **`DownloadReleaseAsset` returns exactly one of `rc` and `redirectURL`.** Assets live on a CDN, so
   the API answers 302. Passing a non-nil `followRedirects` client gets you the body; passing `nil`
   gets you the URL and a **nil reader**. Check `rc != nil` before using it, or nil-deref.
3. **It reads the whole body itself for JSON endpoints** — there is no size cap. Bound anything you
   read yourself (asset downloads) with `io.LimitReader`.
4. **`GetLatestRelease` is not "the newest tag".** GitHub's "latest" excludes prereleases and drafts
   and can be set by hand. For channel-aware selection, list and choose yourself.
5. **Errors are `*github.ErrorResponse`**, carrying the `*http.Response`. Use `errors.As` to reach the
   status code. Rate limiting has its own types: `*RateLimitError` (primary) and
   `*AbuseRateLimitError` (secondary, with `RetryAfter`).
6. **A 404 is what a private repository looks like** to a client that cannot see it — GitHub does not
   distinguish it from a missing one.
7. **No timeout by default.** `github.NewClient(nil)` uses `http.DefaultClient`, which waits forever.
   Always pass an `*http.Client` with a `Timeout`.
8. **`ReleaseAsset.Digest`** (`sha256:...`) is populated for recent releases and may be empty for
   older ones. It is a per-asset digest served by the same API as the asset, so it guards corruption,
   not authenticity — the same trust model as goreleaser's `checksums.txt`.
9. **Pagination is manual.** `resp.NextPage == 0` means the last page. cwm asks for one page of 30
   releases and never paginates: releases come back newest first and one page is more than enough.

## Project conventions

- `internal/update.Client` is the only thing that touches this library. It converts
  `*github.RepositoryRelease` into `update.Release` at the boundary, so pointer-field handling and
  the `GetXxx()` discipline live in one file and the rest of cwm sees plain values.
- Construct with `ClientOptions`, never a bare `github.NewClient` elsewhere.
- cwm reads public releases anonymously. There is no token plumbing: if authentication is ever
  needed, `WithAuthToken` is the hook, and it belongs in `NewClient`.
- `exhaustruct_v5` applies to this library's structs. `github.ListOptions` has two fields; set both.
  If a wider struct literal is ever needed, extend the `text:` exclusion in `.golangci.yml` rather
  than adding per-literal directives.

## Omitted

Everything but releases: webhooks, Apps/installation auth, GraphQL, the other ~50 services, and the
rate-limit sleeping helpers. All exist; none are used yet.
