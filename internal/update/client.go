package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Defaults for talking to GitHub.
const (
	// DefaultAPIBaseURL is GitHub's REST API root.
	DefaultAPIBaseURL = "https://api.github.com"
	// DefaultRepo is where cwm's releases are published.
	DefaultRepo = "pdylanross/claude-workspace-manager"
)

// Limits on what cwm will read from the network. Every response is bounded:
// this code runs unattended on someone's machine and then executes what it
// downloaded, so an unbounded read is not acceptable even from a trusted host.
const (
	// releasePageSize is how many releases one listing asks for. They come back
	// newest first, so this is far more than enough to find the latest on
	// either channel.
	releasePageSize = 30
	// maxListingBytes caps the release listing.
	maxListingBytes = 8 << 20
	// maxChecksumsBytes caps the checksums file.
	maxChecksumsBytes = 1 << 20
	// maxArchiveBytes caps a release archive. A cwm build is a few megabytes.
	maxArchiveBytes = 128 << 20
)

// ErrRequestFailed is returned when GitHub answers with something other than
// success.
var ErrRequestFailed = errors.New("github request failed")

// ErrTooLarge is returned when a response exceeds the limit for its kind.
var ErrTooLarge = errors.New("response is larger than expected")

// Client reads releases and their assets from GitHub.
type Client struct {
	httpClient *http.Client
	baseURL    string
	repo       string
	userAgent  string
}

// NewClient returns a Client reading repo ("owner/name") from the API at
// baseURL. userAgent identifies cwm to GitHub, which requires one.
func NewClient(httpClient *http.Client, baseURL, repo, userAgent string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		repo:       repo,
		userAgent:  userAgent,
	}
}

// Releases returns the most recent releases, newest first, with drafts left
// out. A draft is not published, so from cwm's side it does not exist.
func (c *Client) Releases(ctx context.Context) ([]Release, error) {
	endpoint, err := url.JoinPath(c.baseURL, "repos", c.repo, "releases")
	if err != nil {
		return nil, fmt.Errorf("build the releases url: %w", err)
	}

	endpoint += "?per_page=" + strconv.Itoa(releasePageSize)

	body, err := c.get(ctx, endpoint, "application/vnd.github+json", maxListingBytes)
	if err != nil {
		return nil, err
	}

	var payload []releasePayload
	if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
		return nil, fmt.Errorf("parse the release listing: %w", decodeErr)
	}

	releases := make([]Release, 0, len(payload))

	for _, entry := range payload {
		if entry.Draft {
			continue
		}

		releases = append(releases, entry.release())
	}

	return releases, nil
}

// Checksums downloads a release's checksums file.
func (c *Client) Checksums(ctx context.Context, asset Asset) ([]byte, error) {
	return c.get(ctx, asset.URL, "application/octet-stream", maxChecksumsBytes)
}

// Archive downloads a release archive.
func (c *Client) Archive(ctx context.Context, asset Asset) ([]byte, error) {
	return c.get(ctx, asset.URL, "application/octet-stream", maxArchiveBytes)
}

// get fetches a URL and returns at most limit bytes of its body.
func (c *Client) get(ctx context.Context, endpoint, accept string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build a request for %s: %w", endpoint, err)
	}

	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set("X-Github-Api-Version", "2022-11-28")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s returned %s", ErrRequestFailed, endpoint, response.Status)
	}

	// One byte past the limit, so that hitting it is distinguishable from a
	// body that merely fills it.
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read the response from %s: %w", endpoint, err)
	}

	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %s exceeded %d bytes", ErrTooLarge, endpoint, limit)
	}

	return body, nil
}

// releasePayload is the part of GitHub's release representation cwm reads.
type releasePayload struct {
	TagName    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []assetPayload `json:"assets"`
}

// assetPayload is the part of GitHub's asset representation cwm reads.
type assetPayload struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// release converts the payload into cwm's own shape.
func (p releasePayload) release() Release {
	assets := make([]Asset, 0, len(p.Assets))
	for _, asset := range p.Assets {
		assets = append(assets, Asset(asset))
	}

	return Release{Tag: p.TagName, Prerelease: p.Prerelease, Assets: assets}
}
