// Package config defines the cwm configuration document and encodes it as JSON.
//
// The document lives at config.json inside cwm's config root, and cwm owns it:
// it is written by cwm's own commands rather than hand-edited, which is why the
// format is JSON and not something friendlier to write by hand. Everything cwm
// needs is either present in the document or supplied by [Default], so a config
// root that has never been written to is not an error — the first read creates
// the document from the defaults.
package config

import (
	"encoding/json"
	"fmt"
)

// FileName is the name of the config document inside the config root.
const FileName = "config.json"

// jsonIndent is the indentation Save writes, and therefore the indentation a
// hand-inspected document is expected to have.
const jsonIndent = "  "

// Config is the cwm configuration document.
//
// It is deliberately empty for now: the document, its location, and the
// read/write path around it are settled first so that adding a field later is
// only a field. The zero value is the same as [Default].
//
// Every field added here needs a json tag, because the tag — not the Go field
// name — is the on-disk name and is what cwm promises to keep stable.
type Config struct{}

// Default returns the configuration cwm uses when no document exists yet.
//
// This is the opinionated starting point that gets written to disk on first
// use, not a set of fallbacks applied to a partially filled document: a field
// missing from an existing document reads back as its Go zero value.
func Default() Config {
	return Config{}
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
