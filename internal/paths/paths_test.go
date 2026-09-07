package paths_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/paths"
)

// fakeEnv is an in-memory [paths.Environment] so tests never touch the real
// process environment and can therefore run in parallel.
type fakeEnv struct {
	vars      map[string]string
	configDir string
	cacheDir  string
	homeDir   string
	configErr error
	cacheErr  error
	homeErr   error
}

func (f fakeEnv) LookupEnv(key string) (string, bool) {
	value, ok := f.vars[key]

	return value, ok
}

func (f fakeEnv) UserConfigDir() (string, error) { return f.configDir, f.configErr }

func (f fakeEnv) UserCacheDir() (string, error) { return f.cacheDir, f.cacheErr }

func (f fakeEnv) UserHomeDir() (string, error) { return f.homeDir, f.homeErr }

func TestResolverRoots(t *testing.T) {
	t.Parallel()

	relative, absErr := filepath.Abs(filepath.Join("some", "relative", "dir"))
	if absErr != nil {
		t.Fatalf("filepath.Abs() error = %v", absErr)
	}

	tests := []struct {
		name string
		vars map[string]string
		root func(r *paths.Resolver) (string, error)
		want string
	}{
		{
			name: "config root defaults under the OS config dir",
			root: (*paths.Resolver).ConfigRoot,
			want: filepath.Join("/home/u/.config", paths.AppName),
		},
		{
			name: "cache root defaults under the OS cache dir",
			root: (*paths.Resolver).CacheRoot,
			want: filepath.Join("/home/u/.cache", paths.AppName),
		},
		{
			name: "config root override wins",
			vars: map[string]string{paths.ConfigRootEnv: "/srv/cwm-config"},
			root: (*paths.Resolver).ConfigRoot,
			want: "/srv/cwm-config",
		},
		{
			name: "cache root override wins",
			vars: map[string]string{paths.CacheRootEnv: "/srv/cwm-cache"},
			root: (*paths.Resolver).CacheRoot,
			want: "/srv/cwm-cache",
		},
		{
			name: "the app name is not appended to an override",
			vars: map[string]string{paths.ConfigRootEnv: "/srv/cwm-config"},
			root: (*paths.Resolver).ConfigRoot,
			want: "/srv/cwm-config",
		},
		{
			name: "an empty override is ignored",
			vars: map[string]string{paths.ConfigRootEnv: ""},
			root: (*paths.Resolver).ConfigRoot,
			want: filepath.Join("/home/u/.config", paths.AppName),
		},
		{
			name: "a whitespace override is ignored",
			vars: map[string]string{paths.CacheRootEnv: "   "},
			root: (*paths.Resolver).CacheRoot,
			want: filepath.Join("/home/u/.cache", paths.AppName),
		},
		{
			name: "a relative override is made absolute",
			vars: map[string]string{paths.ConfigRootEnv: filepath.Join("some", "relative", "dir")},
			root: (*paths.Resolver).ConfigRoot,
			want: relative,
		},
		{
			name: "the cache override does not affect the config root",
			vars: map[string]string{paths.CacheRootEnv: "/srv/cwm-cache"},
			root: (*paths.Resolver).ConfigRoot,
			want: filepath.Join("/home/u/.config", paths.AppName),
		},
		{
			name: "the config override does not affect the cache root",
			vars: map[string]string{paths.ConfigRootEnv: "/srv/cwm-config"},
			root: (*paths.Resolver).CacheRoot,
			want: filepath.Join("/home/u/.cache", paths.AppName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := paths.New(fakeEnv{
				vars:      tt.vars,
				configDir: "/home/u/.config",
				cacheDir:  "/home/u/.cache",
			})

			got, err := tt.root(resolver)
			if err != nil {
				t.Fatalf("root() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("root() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolverRootsPropagateLookupFailures(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("no home directory")

	tests := []struct {
		name    string
		env     fakeEnv
		root    func(r *paths.Resolver) (string, error)
		wantEnv string
	}{
		{
			name:    "config dir lookup fails",
			env:     fakeEnv{configErr: sentinel},
			root:    (*paths.Resolver).ConfigRoot,
			wantEnv: paths.ConfigRootEnv,
		},
		{
			name:    "cache dir lookup fails",
			env:     fakeEnv{cacheErr: sentinel},
			root:    (*paths.Resolver).CacheRoot,
			wantEnv: paths.CacheRootEnv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.root(paths.New(tt.env))
			if err == nil {
				t.Fatalf("root() = %q, want an error", got)
			}

			if !errors.Is(err, sentinel) {
				t.Errorf("root() error = %v, want it to wrap %v", err, sentinel)
			}

			// The message has to tell the user which variable rescues them.
			if msg := err.Error(); !strings.Contains(msg, tt.wantEnv) {
				t.Errorf("root() error = %q, want it to mention %q", msg, tt.wantEnv)
			}
		})
	}
}

func TestResolverOverrideSurvivesAFailedLookup(t *testing.T) {
	t.Parallel()

	resolver := paths.New(fakeEnv{
		vars:      map[string]string{paths.ConfigRootEnv: "/srv/cwm-config"},
		configErr: errors.New("no home directory"),
	})

	got, err := resolver.ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if want := "/srv/cwm-config"; got != want {
		t.Errorf("ConfigRoot() = %q, want %q", got, want)
	}
}

func TestResolverHomeDir(t *testing.T) {
	t.Parallel()

	resolver := paths.New(fakeEnv{homeDir: "/home/u"})

	got, err := resolver.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir() error = %v", err)
	}

	if want := "/home/u"; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

func TestResolverHomeDirPropagatesFailures(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("no home directory")

	got, err := paths.New(fakeEnv{homeErr: sentinel}).HomeDir()
	if err == nil {
		t.Fatalf("HomeDir() = %q, want an error", got)
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("HomeDir() error = %v, want it to wrap %v", err, sentinel)
	}
}

func TestResolverHomeDirIgnoresTheRootOverrides(t *testing.T) {
	t.Parallel()

	resolver := paths.New(fakeEnv{
		vars:    map[string]string{paths.ConfigRootEnv: "/srv/config", paths.CacheRootEnv: "/srv/cache"},
		homeDir: "/home/u",
	})

	got, err := resolver.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir() error = %v", err)
	}

	if want := "/home/u"; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}
