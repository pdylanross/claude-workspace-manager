// Package config defines the cwm configuration document and encodes it as JSON.
//
// The document lives at config.json inside cwm's config root. Everything cwm
// needs is either present in the document or supplied by [Default], so a config
// root that has never been written to is not an error: the first read creates
// the document from the defaults.
package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// FileName is the name of the config document inside the config root.
const FileName = "config.json"

// SchemaVersion is the format of the document this build reads and writes.
//
// It has nothing to do with the version of cwm itself and moves far more
// slowly. Adding a setting does not change it: an older document simply lacks
// the field and gets the default. It is bumped only for a change an older cwm
// would read wrongly rather than not at all — a renamed or repurposed setting,
// or a value whose meaning changes. A document from a newer schema is refused
// rather than guessed at.
const SchemaVersion = 1

// WorkspaceDirName is the directory, under the user's home directory, that
// [Default] points WorkspaceRoot at.
const WorkspaceDirName = "claude-workspaces"

// jsonIndent is the indentation Save writes, and therefore the indentation a
// hand-inspected document is expected to have.
const jsonIndent = "  "

// Config is the cwm configuration document.
//
// Every field needs a json tag, because the tag — not the Go field name — is
// the on-disk name, and that is what cwm promises to keep stable. The cwm tag
// carries the rest: "path" for a setting holding a filesystem path, which is
// expanded and required to be absolute, and "internal" for a field cwm keeps
// for itself, which is written to the document but is not a setting anyone can
// address.
type Config struct {
	// SchemaVersion records which document format this is. See [SchemaVersion].
	SchemaVersion int `cwm:"internal" json:"schemaVersion"`
	// WorkspaceRoot is the directory cwm keeps workspaces in. Each workspace is
	// a directory beneath it.
	WorkspaceRoot string `cwm:"path" json:"workspaceRoot"`
}

// Default returns the configuration cwm uses when no document exists yet, with
// the paths in it derived from homeDir.
//
// This is the opinionated starting point that gets written to disk on first
// use. It is also what fills in a setting an existing document leaves blank, so
// that a document written by an older cwm — one that had never heard of the
// setting — still reads back as something usable.
//
// homeDir is a parameter rather than a lookup so that this stays a pure
// function; callers get it from [os.UserHomeDir].
func Default(homeDir string) Config {
	return Config{
		SchemaVersion: SchemaVersion,
		WorkspaceRoot: filepath.Join(homeDir, WorkspaceDirName),
	}
}

// Encode returns the JSON encoding of the document, in the same layout Save
// writes: indented, with a trailing newline.
func (c Config) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", jsonIndent)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	return append(data, '\n'), nil
}

// Normalize returns c as cwm would store it: blank settings taken from the
// defaults for homeDir, and every path setting expanded and cleaned.
//
// Expansion is what makes "cwm config set workspaceRoot=~/ws" work. A shell may
// or may not expand the tilde in that argument — bash does, zsh does not — so
// cwm cannot rely on having received an expanded path and does it here.
//
// Normalizing is not the same as validating: a path can survive this and still
// be rejected by [Config.Validate], as a relative one is.
func (c Config) Normalize(homeDir string) Config {
	normalized := c.withDefaults(Default(homeDir))

	// The error is the walk's, and this walk cannot fail.
	_ = eachPathSetting(&normalized, func(_ string, setting *string) error {
		*setting = expandHome(*setting, homeDir)

		return nil
	})

	return normalized
}

// Validate reports whether the document makes sense: a schema this build
// understands, and an absolute path in every path setting.
//
// It is deliberately about the document alone and touches no filesystem. Where
// a directory has to exist, or has to be a directory rather than a file, is a
// question for the code that uses it, not for reading the config.
func (c Config) Validate() error {
	if err := c.validateSchema(); err != nil {
		return err
	}

	return eachPathSetting(&c, func(name string, setting *string) error {
		if strings.TrimSpace(*setting) == "" {
			return fmt.Errorf("%w: %s is empty", ErrInvalidSetting, name)
		}

		if !filepath.IsAbs(*setting) {
			return fmt.Errorf("%w: %s must be an absolute path, but is %q", ErrInvalidSetting, name, *setting)
		}

		return nil
	})
}

// validateSchema reports whether this build understands the document's format.
func (c Config) validateSchema() error {
	if c.SchemaVersion < 1 {
		return fmt.Errorf("%w: the document does not say which format it is", ErrUnsupportedSchema)
	}

	if c.SchemaVersion > SchemaVersion {
		return fmt.Errorf(
			"%w: the document is schema %d and this cwm understands %d; upgrade cwm to read it",
			ErrUnsupportedSchema, c.SchemaVersion, SchemaVersion,
		)
	}

	return nil
}

// withDefaults returns c with every blank setting taken from defaults.
//
// Blankness is decided per setting rather than by a general "is this the zero
// value" rule, because zero is a real answer for most types: a false boolean is
// a choice, and an empty string may be a deliberate "no prefix". Only a setting
// for which unset is meaningless is filled in here.
func (c Config) withDefaults(defaults Config) Config {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = defaults.SchemaVersion
	}

	if strings.TrimSpace(c.WorkspaceRoot) == "" {
		c.WorkspaceRoot = defaults.WorkspaceRoot
	}

	return c
}
