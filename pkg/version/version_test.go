package version_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/pkg/version"
)

func TestInfoShort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info version.Info
		want string
	}{
		{"zero value falls back to dev", version.Info{}, version.DefaultVersion},
		{"blank version falls back to dev", version.Info{Version: "  "}, version.DefaultVersion},
		{"stamped version is used", version.Info{Version: "1.2.3"}, "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.info.Short(); got != tt.want {
				t.Errorf("Short() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInfoString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info version.Info
		want []string
	}{
		{
			name: "zero value renders defaults",
			info: version.Info{},
			want: []string{
				"version:  " + version.DefaultVersion,
				"commit:   " + version.DefaultCommit,
				"built:    " + version.DefaultDate,
			},
		},
		{
			name: "stamped build renders its own fields",
			info: version.Info{Version: "1.2.3", Commit: "abc1234", Date: "2026-09-07T12:00:00Z"},
			want: []string{
				"version:  1.2.3",
				"commit:   abc1234",
				"built:    2026-09-07T12:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.info.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("String() missing %q, got:\n%s", want, got)
				}
			}

			if !strings.Contains(got, runtime.Version()) {
				t.Errorf("String() missing go version %q, got:\n%s", runtime.Version(), got)
			}

			if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
				t.Errorf("String() missing platform, got:\n%s", got)
			}
		})
	}
}
