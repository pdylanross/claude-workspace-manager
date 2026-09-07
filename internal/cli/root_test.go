package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/cli"
	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	info := version.Info{Version: "1.2.3", Commit: "abc1234", Date: "2026-09-07T12:00:00Z"}

	tests := []struct {
		name     string
		args     []string
		want     []string
		exact    bool
		wantText string
	}{
		{
			name: "full output",
			args: []string{"version"},
			want: []string{"1.2.3", "abc1234", "2026-09-07T12:00:00Z"},
		},
		{
			name:     "short output",
			args:     []string{"version", "--short"},
			exact:    true,
			wantText: "1.2.3\n",
		},
		{
			name:     "short shorthand",
			args:     []string{"version", "-s"},
			exact:    true,
			wantText: "1.2.3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			cmd := cli.NewRootCmd(info)
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			got := out.String()

			if tt.exact {
				if got != tt.wantText {
					t.Errorf("output = %q, want %q", got, tt.wantText)
				}

				return
			}

			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q, got:\n%s", want, got)
				}
			}
		})
	}
}

func TestVersionCommandRejectsArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	cmd := cli.NewRootCmd(version.Info{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version", "unexpected"})

	if err := cmd.Execute(); err == nil {
		t.Error("Execute() error = nil, want an error for an unexpected argument")
	}
}
