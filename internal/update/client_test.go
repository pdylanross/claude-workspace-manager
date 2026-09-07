package update_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/update"
)

// releasesJSON is a listing in GitHub's shape, including a draft that must be
// filtered out and a release with no assets.
const releasesJSON = `[
  {"tag_name":"v2.0.0","draft":true,"prerelease":false,"assets":[]},
  {"tag_name":"v1.3.0","draft":false,"prerelease":true,
   "assets":[{"name":"cwm_1.3.0_linux_amd64.tar.gz","browser_download_url":"https://example.test/a"}]},
  {"tag_name":"v1.2.0","draft":false,"prerelease":false,
   "assets":[{"name":"checksums.txt","browser_download_url":"https://example.test/c"}]}
]`

// newTestClient returns a client pointed at a server running handler, along
// with that server's URL for the asset downloads that take one directly.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*update.Client, string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return update.NewClient(server.Client(), server.URL, "owner/repo", "cwm-test"), server.URL
}

func TestClientReleases(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if want := "/repos/owner/repo/releases"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}

		if r.URL.Query().Get("per_page") == "" {
			t.Error("request did not ask for a page size")
		}

		if r.Header.Get("User-Agent") != "cwm-test" {
			t.Errorf("User-Agent = %q, want cwm-test", r.Header.Get("User-Agent"))
		}

		if !strings.Contains(r.Header.Get("Accept"), "github") {
			t.Errorf("Accept = %q, want the GitHub media type", r.Header.Get("Accept"))
		}

		_, _ = w.Write([]byte(releasesJSON))
	})

	releases, err := client.Releases(t.Context())
	if err != nil {
		t.Fatalf("Releases() error = %v", err)
	}

	if len(releases) != 2 {
		t.Fatalf("Releases() returned %d releases, want 2 with the draft left out", len(releases))
	}

	if releases[0].Tag != "v1.3.0" || !releases[0].Prerelease {
		t.Errorf("Releases()[0] = %+v, want the v1.3.0 prerelease", releases[0])
	}

	asset, found := releases[0].Asset("cwm_1.3.0_linux_amd64.tar.gz")
	if !found || asset.URL != "https://example.test/a" {
		t.Errorf("Releases()[0] asset = %+v, %t, want the download url", asset, found)
	}
}

func TestClientReleasesRejectsAnErrorStatus(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})

	_, err := client.Releases(t.Context())
	if !errors.Is(err, update.ErrRequestFailed) {
		t.Errorf("Releases() error = %v, want ErrRequestFailed", err)
	}
}

func TestClientReleasesRejectsRubbish(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not json"))
	})

	if _, err := client.Releases(t.Context()); err == nil {
		t.Error("Releases() error = nil, want an error")
	}
}

func TestClientRefusesAnOversizedResponse(t *testing.T) {
	t.Parallel()

	// The checksums file is capped far below what a real one needs; a response
	// far above that is a server misbehaving, and cwm stops reading.
	client, serverURL := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		flood := strings.Repeat("x", 1<<20)
		for range 4 {
			_, _ = w.Write([]byte(flood))
		}
	})

	_, err := client.Checksums(t.Context(), update.Asset{Name: "checksums.txt", URL: serverURL})
	if !errors.Is(err, update.ErrTooLarge) {
		t.Errorf("Checksums() error = %v, want ErrTooLarge", err)
	}
}

func TestClientHonoursACancelledContext(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releasesJSON))
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.Releases(ctx); err == nil {
		t.Error("Releases() error = nil, want a context error")
	}
}
