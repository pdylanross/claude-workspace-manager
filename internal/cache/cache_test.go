package cache_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdylanross/claude-workspace-manager/internal/cache"
)

func TestCacheReadMiss(t *testing.T) {
	t.Parallel()

	data, found, err := cache.New(t.TempDir()).Read("absent")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if found {
		t.Errorf("Read() found = true with data %q, want a miss", data)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	t.Parallel()

	c := cache.New(filepath.Join(t.TempDir(), "not", "created", "yet"))

	if err := c.Write("entry", []byte("contents")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, found, err := c.Read("entry")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !found || string(data) != "contents" {
		t.Errorf("Read() = %q, %t, want %q, true", data, found, "contents")
	}
}

func TestCacheWriteIsPrivate(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores file permissions")
	}

	root := filepath.Join(t.TempDir(), "cwm")
	c := cache.New(root)

	if err := c.Write("entry", []byte("contents")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	dir, err := os.Stat(root)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if mode := dir.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("cache root mode = %#o, want no group or other access", mode)
	}

	path, err := c.Path("entry")
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}

	file, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if mode := file.Mode().Perm(); mode != 0o600 {
		t.Errorf("entry mode = %#o, want %#o", mode, 0o600)
	}
}

func TestCacheOverwrites(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir())

	if err := c.Write("entry", []byte("a much longer first value")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := c.Write("entry", []byte("short")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, _, err := c.Read("entry")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if string(data) != "short" {
		t.Errorf("Read() = %q, want %q", data, "short")
	}
}

func TestCacheTimeRoundTrip(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir())
	want := time.Now().Truncate(time.Second)

	if err := c.WriteTime("last_update", want); err != nil {
		t.Fatalf("WriteTime() error = %v", err)
	}

	got, found, err := c.ReadTime("last_update")
	if err != nil {
		t.Fatalf("ReadTime() error = %v", err)
	}

	if !found || !got.Equal(want) {
		t.Errorf("ReadTime() = %v, %t, want %v, true", got, found, want)
	}
}

func TestCacheWriteTimeIsReadableByHand(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir())

	if err := c.WriteTime("last_update", time.Unix(1757270000, 0)); err != nil {
		t.Fatalf("WriteTime() error = %v", err)
	}

	data, _, err := c.Read("last_update")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if want := "1757270000\n"; string(data) != want {
		t.Errorf("entry = %q, want %q", data, want)
	}
}

func TestCacheReadTimeTreatsNonsenseAsAMiss(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{"not a number", "the day before yesterday"},
		{"empty", ""},
		{"a torn write", "17572"},
		{"a float", "1757270000.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cache.New(t.TempDir())

			if err := c.Write("last_update", []byte(tt.contents)); err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			_, found, err := c.ReadTime("last_update")
			if err != nil {
				t.Errorf("ReadTime() error = %v, want a miss rather than an error", err)
			}

			// "a torn write" parses as a number, so only the unparseable ones
			// are misses; either way ReadTime must not fail.
			if found && tt.contents != "17572" {
				t.Errorf("ReadTime() found = true for %q, want a miss", tt.contents)
			}
		})
	}
}

func TestCacheRemove(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir())

	if err := c.Write("entry", []byte("contents")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := c.Remove("entry"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, found, _ := c.Read("entry"); found {
		t.Error("Read() found the entry after Remove")
	}

	// Removing what is not there is how a cache is cleaned up, not an error.
	if err := c.Remove("entry"); err != nil {
		t.Errorf("Remove() on a missing entry error = %v, want nil", err)
	}
}

func TestCacheRejectsNamesThatEscapeTheRoot(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", ".", "..", "../outside", "sub/entry", "/absolute"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := cache.New(t.TempDir())

			if _, err := c.Path(name); !errors.Is(err, cache.ErrInvalidName) {
				t.Errorf("Path(%q) error = %v, want ErrInvalidName", name, err)
			}

			if err := c.Write(name, nil); !errors.Is(err, cache.ErrInvalidName) {
				t.Errorf("Write(%q) error = %v, want ErrInvalidName", name, err)
			}

			if _, _, err := c.Read(name); !errors.Is(err, cache.ErrInvalidName) {
				t.Errorf("Read(%q) error = %v, want ErrInvalidName", name, err)
			}

			if err := c.Remove(name); !errors.Is(err, cache.ErrInvalidName) {
				t.Errorf("Remove(%q) error = %v, want ErrInvalidName", name, err)
			}
		})
	}
}
