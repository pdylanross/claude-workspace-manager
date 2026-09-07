package paths_test

import (
	"os"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
)

func TestOSEnvironmentReadsTheProcess(t *testing.T) {
	t.Parallel()

	env := paths.NewOSEnvironment()

	wantConfig, wantConfigErr := os.UserConfigDir()

	gotConfig, err := env.UserConfigDir()
	if (err != nil) != (wantConfigErr != nil) {
		t.Fatalf("UserConfigDir() error = %v, want an error = %v", err, wantConfigErr != nil)
	}

	if err == nil && gotConfig != wantConfig {
		t.Errorf("UserConfigDir() = %q, want %q", gotConfig, wantConfig)
	}

	wantCache, wantCacheErr := os.UserCacheDir()

	gotCache, err := env.UserCacheDir()
	if (err != nil) != (wantCacheErr != nil) {
		t.Fatalf("UserCacheDir() error = %v, want an error = %v", err, wantCacheErr != nil)
	}

	if err == nil && gotCache != wantCache {
		t.Errorf("UserCacheDir() = %q, want %q", gotCache, wantCache)
	}

	wantValue, wantOK := os.LookupEnv("PATH")

	gotValue, gotOK := env.LookupEnv("PATH")
	if gotValue != wantValue || gotOK != wantOK {
		t.Errorf("LookupEnv(PATH) = %q, %t, want %q, %t", gotValue, gotOK, wantValue, wantOK)
	}

	if _, ok := env.LookupEnv("CWM_DEFINITELY_NOT_SET_43F1"); ok {
		t.Error("LookupEnv() reported an unset variable as set")
	}
}
