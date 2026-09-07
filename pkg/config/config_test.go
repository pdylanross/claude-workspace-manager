package config_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
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

func TestConfigShapeIsAddressable(t *testing.T) {
	t.Parallel()

	// Guards the rule in the code-standards skill: every setting has to be
	// reachable by path, so no slices, arrays or maps in the document.
	if err := config.CheckShape(); err != nil {
		t.Errorf("CheckShape() error = %v, want nil", err)
	}
}

func TestSettings(t *testing.T) {
	t.Parallel()

	got := config.Settings()
	if len(got) == 0 {
		t.Fatal("Settings() is empty, want at least one setting")
	}

	if !slices.Contains(got, "workspaceRoot") {
		t.Errorf("Settings() = %v, want it to include workspaceRoot", got)
	}

	// Every listed setting has to be addressable, or the listing is a lie.
	for _, setting := range got {
		if _, err := config.Default("/home/tester").Get(setting); err != nil {
			t.Errorf("Get(%q) error = %v, want every listed setting to be addressable", setting, err)
		}
	}
}

func TestConfigGet(t *testing.T) {
	t.Parallel()

	cfg := config.Default("/home/tester")

	got, err := cfg.Get("workspaceRoot")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got != cfg.WorkspaceRoot {
		t.Errorf("Get() = %v, want %q", got, cfg.WorkspaceRoot)
	}
}

func TestConfigSet(t *testing.T) {
	t.Parallel()

	original := config.Default("/home/tester")

	updated, err := original.Set("workspaceRoot", "/srv/workspaces")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if want := "/srv/workspaces"; updated.WorkspaceRoot != want {
		t.Errorf("Set() left WorkspaceRoot = %q, want %q", updated.WorkspaceRoot, want)
	}

	// Set works on a copy: a caller applying several settings can throw the
	// whole result away when one of them fails.
	if original.WorkspaceRoot == updated.WorkspaceRoot {
		t.Error("Set() modified the receiver, want it left alone")
	}
}

func TestConfigSetRejectsUnknownSettings(t *testing.T) {
	t.Parallel()

	_, err := config.Default("/home/tester").Set("nope", "x")
	if err == nil {
		t.Fatal("Set() error = nil, want an error")
	}

	if !errors.Is(err, config.ErrUnknownSetting) {
		t.Errorf("Set() error = %v, want it to wrap ErrUnknownSetting", err)
	}
}

func TestConfigWithDefaults(t *testing.T) {
	t.Parallel()

	defaults := config.Default("/home/tester")

	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"a blank setting takes the default", config.Config{WorkspaceRoot: ""}, defaults.WorkspaceRoot},
		{"whitespace counts as blank", config.Config{WorkspaceRoot: "  "}, defaults.WorkspaceRoot},
		{"a set value is kept", config.Config{WorkspaceRoot: "/srv/ws"}, "/srv/ws"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.cfg.WithDefaults(defaults).WorkspaceRoot; got != tt.want {
				t.Errorf("WithDefaults().WorkspaceRoot = %q, want %q", got, tt.want)
			}
		})
	}
}
