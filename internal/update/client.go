package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v76/github"
)

// DefaultRepo is where cwm's releases are published.
const DefaultRepo = "pdylanross/claude-workspace-manager"

// releasePageSize is how many releases one listing asks for. They come back
// newest first, so this is far more than enough to find the latest on either
// channel, and cwm never paginates.
const releasePageSize = 30

// Limits on what cwm will read from the network. An asset download is read by
// cwm rather than by the library, so cwm bounds it: this code runs unattended
// and then executes what it downloaded.
const (
	// maxChecksumsBytes caps the checksums file.
	maxChecksumsBytes = 1 << 20
	// maxArchiveBytes caps a release archive. A cwm build is a few megabytes.
	maxArchiveBytes = 128 << 20
)

// ErrRequestFailed is returned when GitHub cannot be read. It wraps the
// library's own error, so a caller that wants the status code can still reach
// it with [errors.As].
var ErrRequestFailed = errors.New("github request failed")

// ErrTooLarge is returned when a download exceeds the limit for its kind.
var ErrTooLarge = errors.New("download is larger than expected")

// ErrInvalidRepo is returned for a repository not written as "owner/name".
var ErrInvalidRepo = errors.New("repository must be written as owner/name")

// ClientOptions is what a [Client] is built from.
type ClientOptions struct {
	// HTTPClient carries the timeout and is required: the library's default
	// client has none and would wait forever.
	HTTPClient *http.Client
	// BaseURL is the API root. Empty means github.com.
	BaseURL string
	// Repo is "owner/name".
	Repo string
	// UserAgent identifies cwm, which GitHub requires.
	UserAgent string
}

// Client reads releases and their assets from GitHub.
//
// It is the only part of cwm that touches the GitHub library: releases are
// converted to cwm's own types here, so the library's all-pointer fields are
// handled in one place and the rest of cwm sees plain values.
type Client struct {
	api       *github.Client
	downloads *http.Client
	owner     string
	repo      string
}

// NewClient returns a Client reading releases as described by options.
func NewClient(options ClientOptions) (*Client, error) {
	owner, name, ok := strings.Cut(options.Repo, "/")
	if !ok || owner == "" || name == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidRepo, options.Repo)
	}

	api := github.NewClient(options.HTTPClient)
	api.UserAgent = options.UserAgent

	if options.BaseURL != "" {
		// The library resolves request paths against this, and does it wrongly
		// without the trailing slash.
		parsed, err := url.Parse(strings.TrimSuffix(options.BaseURL, "/") + "/")
		if err != nil {
			return nil, fmt.Errorf("parse the api base url %q: %w", options.BaseURL, err)
		}

		api.BaseURL = parsed
	}

	return &Client{api: api, downloads: options.HTTPClient, owner: owner, repo: name}, nil
}

// Releases returns the most recent releases, newest first, with drafts left
// out. A draft is not published, so from cwm's side it does not exist.
func (c *Client) Releases(ctx context.Context) ([]Release, error) {
	published, _, err := c.api.Repositories.ListReleases(ctx, c.owner, c.repo, &github.ListOptions{
		Page:    0,
		PerPage: releasePageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: list releases for %s/%s: %w", ErrRequestFailed, c.owner, c.repo, err)
	}

	releases := make([]Release, 0, len(published))

	for _, entry := range published {
		if entry.GetDraft() {
			continue
		}

		releases = append(releases, releaseFrom(entry))
	}

	return releases, nil
}

// Checksums downloads a release's checksums file.
func (c *Client) Checksums(ctx context.Context, asset Asset) ([]byte, error) {
	return c.download(ctx, asset, maxChecksumsBytes)
}

// Archive downloads a release archive.
func (c *Client) Archive(ctx context.Context, asset Asset) ([]byte, error) {
	return c.download(ctx, asset, maxArchiveBytes)
}

// download fetches an asset and returns at most limit bytes of it.
func (c *Client) download(ctx context.Context, asset Asset, limit int64) ([]byte, error) {
	// Assets live on a CDN, so the API answers with a redirect; handing it a
	// client is what makes it follow rather than hand back a URL and a nil
	// reader.
	reader, redirect, err := c.api.Repositories.DownloadReleaseAsset(ctx, c.owner, c.repo, asset.ID, c.downloads)
	if err != nil {
		return nil, fmt.Errorf("%w: download %s: %w", ErrRequestFailed, asset.Name, err)
	}

	if reader == nil {
		return nil, fmt.Errorf(
			"%w: download %s: redirected to %s and not followed",
			ErrRequestFailed,
			asset.Name,
			redirect,
		)
	}

	defer func() { _ = reader.Close() }()

	// One byte past the limit, so that hitting it is distinguishable from a
	// download that merely fills it.
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", asset.Name, err)
	}

	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %s exceeded %d bytes", ErrTooLarge, asset.Name, limit)
	}

	return body, nil
}

// releaseFrom converts a published release into cwm's own shape.
//
// Every field of the library's type is a pointer, so this goes through the
// generated accessors: a field GitHub happened to omit reads as its zero value
// rather than panicking.
func releaseFrom(published *github.RepositoryRelease) Release {
	assets := make([]Asset, 0, len(published.Assets))
	for _, asset := range published.Assets {
		assets = append(assets, Asset{Name: asset.GetName(), ID: asset.GetID()})
	}

	return Release{
		Tag:        published.GetTagName(),
		Prerelease: published.GetPrerelease(),
		Assets:     assets,
	}
}
