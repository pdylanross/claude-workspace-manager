package config_test

import (
	"encoding/json"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

func TestDefaultMatchesTheZeroValue(t *testing.T) {
	t.Parallel()

	if got := config.Default(); got != (config.Config{}) {
		t.Errorf("Default() = %+v, want the zero value %+v", got, config.Config{})
	}
}

func TestConfigEncode(t *testing.T) {
	t.Parallel()

	data, err := config.Default().Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if want := "{}\n"; string(data) != want {
		t.Errorf("Encode() = %q, want %q", data, want)
	}
}

func TestConfigRoundTrips(t *testing.T) {
	t.Parallel()

	data, err := config.Default().Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got config.Config
	if decodeErr := json.Unmarshal(data, &got); decodeErr != nil {
		t.Fatalf("json.Unmarshal() error = %v", decodeErr)
	}

	if got != config.Default() {
		t.Errorf("round trip = %+v, want %+v", got, config.Default())
	}
}
