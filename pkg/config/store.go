package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Permissions for the config root and the document inside it. Both are
// restrictive because the document is expected to grow fields that are nobody
// else's business.
const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// Store reads and writes the config document under one config root.
//
// A Store holds no state beyond that root: it does not cache the document, so
// every Load observes what is actually on disk.
type Store struct {
	root string
}

// NewStore returns a Store for the config document under root, which should be
// an absolute path. The root is not created until something is written.
func NewStore(root string) *Store {
	return &Store{root: root}
}

// Path returns the path of the config document.
func (s *Store) Path() string {
	return filepath.Join(s.root, FileName)
}

// Exists reports whether the config document is present on disk.
//
// A missing document is not an error; it is the state every config root starts
// in and the one Load resolves by writing the defaults.
func (s *Store) Exists() (bool, error) {
	_, err := os.Stat(s.Path())

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("inspect %s: %w", s.Path(), err)
	}
}

// Load reads the config document.
//
// When the document does not exist, Load writes [Default] to disk and returns
// it, so that the file a user is later told to look at is really there. Fields
// absent from an existing document are left at their Go zero value; the
// defaults are not merged into a document that already exists.
func (s *Store) Load(ctx context.Context) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, fmt.Errorf("load %s: %w", s.Path(), err)
	}

	data, readErr := os.ReadFile(s.Path())

	if errors.Is(readErr, fs.ErrNotExist) {
		cfg := Default()
		if saveErr := s.Save(ctx, cfg); saveErr != nil {
			return Config{}, saveErr
		}

		return cfg, nil
	}

	if readErr != nil {
		return Config{}, fmt.Errorf("read %s: %w", s.Path(), readErr)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", s.Path(), err)
	}

	return cfg, nil
}

// Save writes cfg to the config document, creating the config root if it is
// missing.
//
// The write is atomic: cfg goes to a temporary file in the config root and is
// then renamed over the document, so an interrupted write cannot leave a
// truncated config behind.
func (s *Store) Save(ctx context.Context, cfg Config) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save %s: %w", s.Path(), err)
	}

	if err := os.MkdirAll(s.root, dirPerm); err != nil {
		return fmt.Errorf("create config root %s: %w", s.root, err)
	}

	data, encErr := cfg.Encode()
	if encErr != nil {
		return encErr
	}

	if err := writeAtomic(s.root, s.Path(), data); err != nil {
		return fmt.Errorf("write %s: %w", s.Path(), err)
	}

	return nil
}

// writeAtomic writes data to path by way of a temporary file in dir, which must
// be the directory holding path so that the rename stays within one filesystem.
func writeAtomic(dir, path string, data []byte) error {
	tmp, createErr := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if createErr != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, createErr)
	}

	name := tmp.Name()

	// A no-op once the rename below has succeeded, and the cleanup otherwise.
	defer func() { _ = os.Remove(name) }()

	// os.CreateTemp already opens at 0600; this pins the mode rather than
	// inheriting whatever the standard library happens to use.
	if chmodErr := tmp.Chmod(filePerm); chmodErr != nil {
		_ = tmp.Close()

		return fmt.Errorf("set the permissions of %s: %w", name, chmodErr)
	}

	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()

		return fmt.Errorf("write %s: %w", name, writeErr)
	}

	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("close %s: %w", name, closeErr)
	}

	if renameErr := os.Rename(name, path); renameErr != nil {
		return fmt.Errorf("rename %s to %s: %w", name, path, renameErr)
	}

	return nil
}
