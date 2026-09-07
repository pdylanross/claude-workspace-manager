// Command cwm manages Claude workspaces.
package main

import (
	"os"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
	buildinfo "github.com/pdylanross/claude-workspace-manager/pkg/version"
)

// Build-time variables injected via ldflags by goreleaser. The names are load
// bearing: .goreleaser.yml stamps them as main.version, main.commit, main.date.
var (
	version = ""
	commit  = ""
	date    = ""
)

func main() {
	info := buildinfo.Info{Version: version, Commit: commit, Date: date}

	if err := cli.Execute(info); err != nil {
		os.Exit(1)
	}
}
