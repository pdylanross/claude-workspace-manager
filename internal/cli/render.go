package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// settingIndent is one level of nesting in the listing.
const settingIndent = "  "

// renderSettings lays a configuration out for a person to read.
func renderSettings(cmd *cobra.Command, cfg config.Config, prefix string) (string, error) {
	// Bound to the command's own writer: the package-level lipgloss helpers
	// read the colour profile from the process's stdout instead, which in a
	// test writing to a buffer means ANSI from a terminal and none in CI.
	styles := newStyles(cmd)

	blocks, err := settingBlocks(cfg, prefix)
	if err != nil {
		return "", err
	}

	var (
		out      strings.Builder
		previous []string
	)

	for _, b := range blocks {
		out.WriteString(renderHeadings(styles, previous, b.group, out.Len() > 0))
		out.WriteString(renderRows(styles, b.rows, len(b.group)))

		previous = b.group
	}

	return out.String(), nil
}

// block is a run of settings sharing a group.
type block struct {
	group []string
	rows  []row
}

// row is one setting on its way to being printed.
type row struct {
	name  string
	value string
}

// styles is the small palette the configuration listing uses.
type styles struct {
	group lipgloss.Style
	name  lipgloss.Style
	value lipgloss.Style
	empty lipgloss.Style
}

// newStyles builds a palette against the command's output, so that colour is
// decided by where the text is actually going.
func newStyles(cmd *cobra.Command) styles {
	renderer := lipgloss.NewRenderer(cmd.OutOrStdout())

	return styles{
		group: renderer.NewStyle().Bold(true),
		name:  renderer.NewStyle().Faint(true),
		value: renderer.NewStyle(),
		empty: renderer.NewStyle().Faint(true).Italic(true),
	}
}

// settingBlocks collects the settings worth showing, in declaration order,
// grouped by the path they sit under.
//
// The listing is driven by [config.Settings], so a setting added to the
// document appears here without anyone remembering to add it. What is written
// by hand is only which settings are worth showing — see [hiddenSetting].
func settingBlocks(cfg config.Config, prefix string) ([]block, error) {
	var blocks []block

	for _, setting := range config.Settings() {
		if !strings.HasPrefix(setting, prefix) || hiddenSetting(setting, cfg) {
			continue
		}

		value, err := cfg.Get(setting)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", setting, err)
		}

		group, leaf := splitSetting(setting)
		entry := row{name: leaf, value: fmt.Sprint(value)}

		if len(blocks) > 0 && slices.Equal(blocks[len(blocks)-1].group, group) {
			blocks[len(blocks)-1].rows = append(blocks[len(blocks)-1].rows, entry)

			continue
		}

		blocks = append(blocks, block{group: group, rows: []row{entry}})
	}

	return blocks, nil
}

// renderHeadings writes the group headings that changed since the last block,
// each indented to its depth. A heading already written is not written again.
func renderHeadings(s styles, previous, group []string, anythingWritten bool) string {
	var out strings.Builder

	for depth, segment := range group {
		if depth < len(previous) && previous[depth] == segment {
			continue
		}

		// A blank line before each top-level group, but not at the very top.
		if depth == 0 && anythingWritten {
			out.WriteString("\n")
		}

		fmt.Fprintf(&out, "%s%s\n", strings.Repeat(settingIndent, depth), s.group.Render(segment))
	}

	return out.String()
}

// renderRows writes the settings of one group, aligned, at the given depth.
func renderRows(s styles, rows []row, depth int) string {
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r.name))
	}

	var out strings.Builder

	indent := strings.Repeat(settingIndent, depth)

	for _, r := range rows {
		value := s.value.Render(r.value)
		if strings.TrimSpace(r.value) == "" {
			value = s.empty.Render("not set")
		}

		fmt.Fprintf(&out, "%s%s  %s\n", indent, s.name.Render(pad(r.name, width)), value)
	}

	return out.String()
}

// hiddenSetting reports whether a setting is noise in this configuration.
//
// The forge variants are the case this exists for: a cwm configured against
// GitHub carries GitLab settings in its document, because the type holds every
// variant, and showing someone settings that do nothing is worse than showing
// them nothing at all. "cwm config show --json" prints the document as it
// really is.
func hiddenSetting(setting string, cfg config.Config) bool {
	const (
		forgeKind      = "forge.kind"
		githubSettings = "forge.github."
		gitlabSettings = "forge.gitlab."
	)

	// Once a forge is chosen, the group heading under forge already names it,
	// and "kind github" above a "github" heading says the same thing twice. An
	// unconfigured cwm keeps it, because otherwise the forge group would be
	// empty and the one thing worth knowing — that nothing is set up — would
	// be the thing left out.
	if setting == forgeKind {
		return cfg.Forge.Configured()
	}

	switch cfg.Forge.Kind {
	case config.ForgeGitHub:
		return strings.HasPrefix(setting, gitlabSettings)
	case config.ForgeGitLab:
		return strings.HasPrefix(setting, githubSettings)
	case config.ForgeNone:
		return strings.HasPrefix(setting, githubSettings) || strings.HasPrefix(setting, gitlabSettings)
	default:
		return false
	}
}

// splitSetting divides a setting's path into the group segments it sits under
// and the name shown at the end.
func splitSetting(setting string) ([]string, string) {
	segments := strings.Split(setting, ".")

	return segments[:len(segments)-1], segments[len(segments)-1]
}

// pad right-aligns a column.
func pad(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}
