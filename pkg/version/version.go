// Package version carries the build information stamped into a cwm binary at
// link time and renders it for human consumption.
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// Default values used when a field was not stamped in at build time.
const (
	DefaultVersion = "dev"
	DefaultCommit  = "none"
	DefaultDate    = "unknown"
)

// Info describes the provenance of a cwm binary.
//
// The zero value is valid and renders the Default* constants, so a binary built
// with a plain "go build" reports itself as a dev build rather than as blanks.
type Info struct {
	// Version is the semantic version of the release, e.g. "1.2.3".
	Version string
	// Commit is the git SHA the binary was built from.
	Commit string
	// Date is the RFC 3339 build timestamp.
	Date string
}

// Short returns just the version string.
func (i Info) Short() string {
	return orDefault(i.Version, DefaultVersion)
}

// String returns a multi-line description of the build, one "key: value" pair
// per line.
func (i Info) String() string {
	var b strings.Builder

	for _, f := range []struct{ key, value string }{
		{"version", i.Short()},
		{"commit", orDefault(i.Commit, DefaultCommit)},
		{"built", orDefault(i.Date, DefaultDate)},
		{"go", runtime.Version()},
		{"platform", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
	} {
		fmt.Fprintf(&b, "%-9s %s\n", f.key+":", f.value)
	}

	return b.String()
}

// orDefault returns value, or fallback when value is empty or whitespace.
func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}
