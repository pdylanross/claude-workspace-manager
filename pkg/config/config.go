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

// WorkspaceDirName is the directory, under the user's home directory, that
// [Default] points WorkspaceRoot at.
const WorkspaceDirName = "claude-workspaces"

// jsonIndent is the indentation Save writes, and therefore the indentation a
// hand-inspected document is expected to have.
const jsonIndent = "  "

// Config is the cwm configuration document.
//
// Every field needs a json tag, because the tag — not the Go field name — is
// the on-disk name, and that is what cwm promises to keep stable.
type Config struct {
	// WorkspaceRoot is the directory cwm keeps workspaces in. Each workspace is
	// a directory beneath it.
	WorkspaceRoot string `json:"workspaceRoot"`
}

// Default returns the configuration cwm uses when no document exists yet, with
// the paths in it derived from homeDir.
//
// This is the opinionated starting point that gets written to disk on first
// use. It is also what fills in a field an existing document leaves blank, so
// that a document written by an older cwm — one that had never heard of the
// field — still reads back as something usable.
//
// homeDir is a parameter rather than a lookup so that this stays a pure
// function; callers get it from [os.UserHomeDir].
func Default(homeDir string) Config {
	return Config{WorkspaceRoot: filepath.Join(homeDir, WorkspaceDirName)}
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

// WithDefaults returns c with every blank setting taken from defaults.
//
// A setting is blank when the document omitted it or set it to something with
// no meaning, such as an empty or all-whitespace path. cwm has no setting for
// which "unset" is a distinct, useful state, so the default always wins there.
// This is what makes clearing a setting — "cwm config set workspaceRoot=" —
// mean "put it back to the default" rather than "make it empty".
func (c Config) WithDefaults(defaults Config) Config {
	if strings.TrimSpace(c.WorkspaceRoot) == "" {
		c.WorkspaceRoot = defaults.WorkspaceRoot
	}

	return c
}
