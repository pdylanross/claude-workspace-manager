// Package cache stores small files that cwm can do without.
//
// Everything here is disposable by definition: an entry that is missing,
// unreadable, or garbage is a cache miss, not an error, and the caller
// recomputes. That is what lets a user delete the cache root at any time
// without breaking cwm, and it is why writes are neither atomic nor locked —
// the worst outcome of a torn write is one extra recomputation.
package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Permissions for the cache root and the entries inside it.
const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// ErrInvalidName is returned for an entry name that is not a plain file name,
// which is the only thing a cache entry may be: no directories, no traversal.
var ErrInvalidName = errors.New("invalid cache entry name")

// Cache is a flat set of named entries under one directory.
type Cache struct {
	root string
}

// New returns a Cache rooted at root, which should be an absolute path. The
// directory is not created until something is written.
func New(root string) *Cache {
	return &Cache{root: root}
}

// Path returns where the named entry lives, without touching the filesystem.
func (c *Cache) Path(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}

	return filepath.Join(c.root, name), nil
}

// Read returns the contents of an entry. A missing entry reports false rather
// than an error, because not having something yet is the normal state of a
// cache.
func (c *Cache) Read(name string) ([]byte, bool, error) {
	path, err := c.Path(name)
	if err != nil {
		return nil, false, err
	}

	data, readErr := os.ReadFile(path)

	if errors.Is(readErr, fs.ErrNotExist) {
		return nil, false, nil
	}

	if readErr != nil {
		return nil, false, fmt.Errorf("read cache entry %s: %w", path, readErr)
	}

	return data, true, nil
}

// Write stores data under name, creating the cache root if it is missing.
func (c *Cache) Write(name string, data []byte) error {
	path, err := c.Path(name)
	if err != nil {
		return err
	}

	if mkErr := os.MkdirAll(c.root, dirPerm); mkErr != nil {
		return fmt.Errorf("create cache root %s: %w", c.root, mkErr)
	}

	if writeErr := os.WriteFile(path, data, filePerm); writeErr != nil {
		return fmt.Errorf("write cache entry %s: %w", path, writeErr)
	}

	return nil
}

// ReadTime returns the instant stored in an entry by [Cache.WriteTime].
//
// An entry that is missing, empty, or not a number reports false: a cache
// holding nonsense is treated as a cache holding nothing, so a corrupted
// timestamp makes cwm redo the work rather than fail.
func (c *Cache) ReadTime(name string) (time.Time, bool, error) {
	data, found, err := c.Read(name)
	if err != nil || !found {
		return time.Time{}, false, err
	}

	seconds, ok := parseUnix(data)
	if !ok {
		return time.Time{}, false, nil
	}

	return time.Unix(seconds, 0), true, nil
}

// WriteTime stores at as a unix timestamp in seconds, one line of decimal text
// so that the entry is readable and diagnosable by hand.
func (c *Cache) WriteTime(name string, at time.Time) error {
	return c.Write(name, fmt.Appendf(nil, "%d\n", at.Unix()))
}

// Remove deletes an entry. Removing one that is not there is not an error.
func (c *Cache) Remove(name string) error {
	path, err := c.Path(name)
	if err != nil {
		return err
	}

	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
		return fmt.Errorf("remove cache entry %s: %w", path, removeErr)
	}

	return nil
}

// parseUnix reads a unix timestamp in seconds, reporting false for anything
// that is not one. The failure is not an error: see [Cache.ReadTime].
func parseUnix(data []byte) (int64, bool) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}

	return seconds, true
}

// validName reports whether name addresses a single file directly inside the
// cache root.
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: the name is empty", ErrInvalidName)
	}

	if name != filepath.Base(name) || name == "." || name == ".." {
		return fmt.Errorf("%w: %q must be a plain file name", ErrInvalidName, name)
	}

	if strings.ContainsRune(name, os.PathSeparator) || strings.ContainsRune(name, '/') {
		return fmt.Errorf("%w: %q must not contain a path separator", ErrInvalidName, name)
	}

	return nil
}
