package forge

import (
	"slices"
	"strings"
)

// defaultGitLabHost is the public instance, which cwm records as no host at all.
const defaultGitLabHost = "gitlab.com"

// scopesLabel is how both clients introduce the scopes line.
const scopesLabel = "Token scopes:"

// parseScopes reads the scopes out of an "auth status" report.
//
// The line looks like: "- Token scopes: 'gist', 'read:org', 'repo'". This is
// screen-scraping and it is deliberately best-effort: scope reporting is
// advice, so a client that changes its wording should cost a warning, never a
// failure.
func parseScopes(output string) []string {
	for line := range strings.Lines(output) {
		_, list, found := strings.Cut(line, scopesLabel)
		if !found {
			continue
		}

		var scopes []string

		for scope := range strings.SplitSeq(list, ",") {
			trimmed := strings.Trim(strings.TrimSpace(scope), "'\"")
			if trimmed != "" {
				scopes = append(scopes, trimmed)
			}
		}

		return scopes
	}

	return nil
}

// parseGitLabHost reads the instance out of a glab "auth status" report, which
// names each host it knows on its own line.
func parseGitLabHost(output string) string {
	for line := range strings.Lines(output) {
		trimmed := strings.TrimSpace(line)

		host, _, found := strings.Cut(trimmed, ":")
		if !found || !strings.Contains(host, ".") || strings.Contains(host, " ") {
			continue
		}

		if host == defaultGitLabHost {
			return ""
		}

		return host
	}

	return ""
}

// slicesContains keeps the [Status] methods free of an import that reads oddly
// next to them.
func slicesContains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
