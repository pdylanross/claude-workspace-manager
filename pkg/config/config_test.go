package config_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

func TestDefaultDerivesTheWorkspaceRootFromHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		home string
		want string
	}{
		{
			name: "a normal home directory",
			home: "/home/tester",
			want: filepath.Join("/home/tester", config.WorkspaceDirName),
		},
		{
			name: "a trailing separator is cleaned away",
			home: "/home/tester/",
			want: filepath.Join("/home/tester", config.WorkspaceDirName),
		},
		{
			name: "an unknown home directory yields a relative root",
			home: "",
			want: config.WorkspaceDirName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := config.Default(tt.home).WorkspaceRoot; got != tt.want {
				t.Errorf("Default(%q).WorkspaceRoot = %q, want %q", tt.home, got, tt.want)
			}
		})
	}
}

func TestConfigEncode(t *testing.T) {
	t.Parallel()

	data, err := config.Default("/home/tester").Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	want := "{\n  \"workspaceRoot\": \"/home/tester/claude-workspaces\"\n}\n"
	if string(data) != want {
		t.Errorf("Encode() = %q, want %q", data, want)
	}
}

func TestConfigEncodeUsesTheOnDiskFieldNames(t *testing.T) {
	t.Parallel()

	data, err := config.Default("/home/tester").Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	// The json tag, not the Go field name, is the name cwm keeps stable.
	if got := string(data); !strings.Contains(got, `"workspaceRoot"`) {
		t.Errorf("Encode() = %s, want it to use the workspaceRoot tag", got)
	}
}

func TestConfigRoundTrips(t *testing.T) {
	t.Parallel()

	want := config.Default("/home/tester")

	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got config.Config
	if decodeErr := json.Unmarshal(data, &got); decodeErr != nil {
		t.Fatalf("json.Unmarshal() error = %v", decodeErr)
	}

	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}
