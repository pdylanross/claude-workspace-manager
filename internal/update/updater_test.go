package update_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdylanross/claude-workspace-manager/internal/cache"
	"github.com/pdylanross/claude-workspace-manager/internal/update"
	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// fixedClock returns a clock stuck at at.
func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

// releaseServer serves a release listing plus the archive and checksums for one
// release, wired so that the asset URLs point back at itself.
func releaseServer(t *testing.T, tag string, archive []byte) *httptest.Server {
	t.Helper()

	var server *httptest.Server

	version := tag[1:]
	name := platformAsset(version)
	checksums := checksumsFor(map[string][]byte{name: archive})

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/archive":
			_, _ = w.Write(archive)
		case "/checksums":
			_, _ = w.Write(checksums)
		default:
			_, _ = w.Write([]byte(`[{"tag_name":"` + tag + `","draft":false,"prerelease":false,"assets":[` +
				`{"name":"` + name + `","browser_download_url":"` + server.URL + `/archive"},` +
				`{"name":"checksums.txt","browser_download_url":"` + server.URL + `/checksums"}]}]`))
		}
	}))

	t.Cleanup(server.Close)

	return server
}

// newTestUpdater wires an updater against server, a fresh cache, and a clock.
func newTestUpdater(t *testing.T, server *httptest.Server, current string, now time.Time) *update.Updater {
	t.Helper()

	client := update.NewClient(update.ClientOptions{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		Repo:       "owner/repo",
		UserAgent:  "cwm-test",
		Token:      "",
	})

	return update.NewUpdater(client, cache.New(t.TempDir()), current, fixedClock(now))
}

func TestUpdaterUpdatable(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)

	if !newTestUpdater(t, server, "1.0.0", time.Now()).Updatable() {
		t.Error("Updatable() = false for a released build, want true")
	}

	if newTestUpdater(t, server, "dev", time.Now()).Updatable() {
		t.Error("Updatable() = true for a dev build, want false")
	}

	if newTestUpdater(t, server, "0.0.0-SNAPSHOT-abc1234", time.Now()).Updatable() {
		t.Error("Updatable() = true for a snapshot build, want false")
	}
}

func TestUpdaterDue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	server := releaseServer(t, "v1.2.0", nil)

	tests := []struct {
		name  string
		since time.Duration
		want  bool
	}{
		{"just now", 0, false},
		{"an hour ago", time.Hour, false},
		{"almost a day ago", update.CheckInterval - time.Minute, false},
		{"exactly the interval ago", update.CheckInterval, true},
		{"a week ago", 7 * update.CheckInterval, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entries := cache.New(t.TempDir())
			if err := entries.WriteTime(update.CacheEntry, now.Add(-tt.since)); err != nil {
				t.Fatalf("WriteTime() error = %v", err)
			}

			client := update.NewClient(update.ClientOptions{
				HTTPClient: server.Client(),
				BaseURL:    server.URL,
				Repo:       "owner/repo",
				UserAgent:  "cwm-test",
				Token:      "",
			})

			updater := update.NewUpdater(client, entries, "1.0.0", fixedClock(now))
			if got := updater.Due(); got != tt.want {
				t.Errorf("Due() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestUpdaterDueWithoutAPreviousCheck(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)

	if !newTestUpdater(t, server, "1.0.0", time.Now()).Due() {
		t.Error("Due() = false having never checked, want true")
	}
}

func TestUpdaterDueWithAnUnreadableEntry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := cache.New(t.TempDir())

	if err := entries.Write(update.CacheEntry, []byte("the day before yesterday")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	server := releaseServer(t, "v1.2.0", nil)
	client := update.NewClient(update.ClientOptions{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		Repo:       "owner/repo",
		UserAgent:  "cwm-test",
		Token:      "",
	})

	// A cache that cannot be read is a cache that says nothing, and the safe
	// answer to "have we checked lately" is no.
	if !update.NewUpdater(client, entries, "1.0.0", fixedClock(now)).Due() {
		t.Error("Due() = false for an unreadable entry, want true")
	}
}

func TestUpdaterMarkChecked(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	entries := cache.New(t.TempDir())
	server := releaseServer(t, "v1.2.0", nil)
	client := update.NewClient(update.ClientOptions{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		Repo:       "owner/repo",
		UserAgent:  "cwm-test",
		Token:      "",
	})
	updater := update.NewUpdater(client, entries, "1.0.0", fixedClock(now))

	if err := updater.MarkChecked(); err != nil {
		t.Fatalf("MarkChecked() error = %v", err)
	}

	if updater.Due() {
		t.Error("Due() = true straight after MarkChecked, want false")
	}

	at, found, err := entries.ReadTime(update.CacheEntry)
	if err != nil || !found {
		t.Fatalf("ReadTime() = %v, %t, %v", at, found, err)
	}

	if !at.Equal(now) {
		t.Errorf("recorded time = %v, want %v", at, now)
	}
}

func TestUpdaterCheck(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)
	updater := newTestUpdater(t, server, "1.0.0", time.Now())

	release, found, err := updater.Check(t.Context(), config.ChannelStable)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !found || release.Tag != "v1.2.0" {
		t.Errorf("Check() = %q, %t, want v1.2.0, true", release.Tag, found)
	}
}

func TestUpdaterCheckFindsNothingWhenCurrent(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)
	updater := newTestUpdater(t, server, "1.2.0", time.Now())

	_, found, err := updater.Check(t.Context(), config.ChannelStable)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if found {
		t.Error("Check() found a newer release while running the newest")
	}
}

func TestUpdaterApply(t *testing.T) {
	t.Parallel()

	archive := makeArchive(t, entry{name: "cwm", contents: "the new binary", dir: false})
	server := releaseServer(t, "v1.2.0", archive)
	updater := newTestUpdater(t, server, "1.0.0", time.Now())

	release, found, err := updater.Check(t.Context(), config.ChannelStable)
	if err != nil || !found {
		t.Fatalf("Check() = %t, %v", found, err)
	}

	target := filepath.Join(t.TempDir(), "cwm")
	if writeErr := os.WriteFile(target, []byte("the old binary"), 0o755); writeErr != nil {
		t.Fatalf("os.WriteFile() error = %v", writeErr)
	}

	installed, err := update.ApplyTo(t.Context(), updater, release, target)
	if err != nil {
		t.Fatalf("ApplyTo() error = %v", err)
	}

	if installed != target {
		t.Errorf("ApplyTo() = %q, want %q", installed, target)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != "the new binary" {
		t.Errorf("installed binary = %q, want %q", data, "the new binary")
	}
}

func TestUpdaterApplyRefusesATamperedArchive(t *testing.T) {
	t.Parallel()

	archive := makeArchive(t, entry{name: "cwm", contents: "the new binary", dir: false})
	name := platformAsset("1.2.0")

	// The listing publishes a digest for one archive and the download serves
	// another: exactly what an update has to refuse.
	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/archive":
			_, _ = w.Write(makeArchive(t, entry{name: "cwm", contents: "something else", dir: false}))
		case "/checksums":
			_, _ = w.Write(checksumsFor(map[string][]byte{name: archive}))
		default:
			_, _ = w.Write([]byte(`[{"tag_name":"v1.2.0","draft":false,"prerelease":false,"assets":[` +
				`{"name":"` + name + `","browser_download_url":"` + server.URL + `/archive"},` +
				`{"name":"checksums.txt","browser_download_url":"` + server.URL + `/checksums"}]}]`))
		}
	}))

	t.Cleanup(server.Close)

	updater := newTestUpdater(t, server, "1.0.0", time.Now())

	release, found, err := updater.Check(t.Context(), config.ChannelStable)
	if err != nil || !found {
		t.Fatalf("Check() = %t, %v", found, err)
	}

	target := filepath.Join(t.TempDir(), "cwm")
	if writeErr := os.WriteFile(target, []byte("the old binary"), 0o755); writeErr != nil {
		t.Fatalf("os.WriteFile() error = %v", writeErr)
	}

	if _, applyErr := update.ApplyTo(
		t.Context(),
		updater,
		release,
		target,
	); !errors.Is(
		applyErr,
		update.ErrChecksumMismatch,
	) {
		t.Fatalf("ApplyTo() error = %v, want ErrChecksumMismatch", applyErr)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != "the old binary" {
		t.Errorf("binary = %q, want the original left in place", data)
	}
}

func TestUpdaterApplyWithoutABuildForThisPlatform(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)
	updater := newTestUpdater(t, server, "1.0.0", time.Now())

	release := update.Release{
		Tag:        "v1.2.0",
		Prerelease: false,
		Assets:     []update.Asset{{Name: "checksums.txt", URL: server.URL + "/checksums"}},
	}

	target := filepath.Join(t.TempDir(), "cwm")

	_, err := update.ApplyTo(t.Context(), updater, release, target)
	if !errors.Is(err, update.ErrNoAsset) {
		t.Errorf("ApplyTo() error = %v, want ErrNoAsset", err)
	}
}

func TestUpdaterApplyWithoutPublishedChecksums(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "v1.2.0", nil)
	updater := newTestUpdater(t, server, "1.0.0", time.Now())

	release := update.Release{
		Tag:        "v1.2.0",
		Prerelease: false,
		Assets: []update.Asset{
			{Name: platformAsset("1.2.0"), URL: server.URL + "/archive"},
		},
	}

	_, err := update.ApplyTo(t.Context(), updater, release, filepath.Join(t.TempDir(), "cwm"))
	if !errors.Is(err, update.ErrNoChecksum) {
		t.Errorf("ApplyTo() error = %v, want ErrNoChecksum", err)
	}
}
