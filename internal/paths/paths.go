// Package paths resolves the directories cwm keeps its own state in.
//
// cwm stores its configuration under a config root and disposable data under a
// cache root. Both default to the OS-conventional location for an application
// named cwm — on Linux ~/.config/cwm and ~/.cache/cwm — and either can be
// pointed somewhere else with an environment variable.
package paths

import (
	"fmt"
	"path/filepath"
	"strings"
)

// AppName is the directory cwm owns inside the OS config and cache directories.
const AppName = "cwm"

// Environment variables that override a resolved root. A variable that is
// unset, empty, or entirely whitespace is ignored, so exporting an empty value
// is the same as not setting one.
const (
	// ConfigRootEnv overrides the config root.
	ConfigRootEnv = "CWM_CONFIG_ROOT"
	// CacheRootEnv overrides the cache root.
	CacheRootEnv = "CWM_CACHE_ROOT"
)

// Environment supplies the process facts that root resolution depends on.
//
// It exists so resolution can be tested without mutating the real process
// environment. [OSEnvironment] is the implementation backed by the running
// process.
type Environment interface {
	// LookupEnv reports the value of an environment variable, as [os.LookupEnv].
	LookupEnv(key string) (string, bool)
	// UserConfigDir reports the OS user config directory, as [os.UserConfigDir].
	UserConfigDir() (string, error)
	// UserCacheDir reports the OS user cache directory, as [os.UserCacheDir].
	UserCacheDir() (string, error)
	// UserHomeDir reports the current user's home directory, as [os.UserHomeDir].
	UserHomeDir() (string, error)
}

// Resolver resolves cwm's roots against an [Environment].
type Resolver struct {
	env Environment
}

// New returns a Resolver that reads from env, which must not be nil. Pass
// [NewOSEnvironment] for the real process environment.
func New(env Environment) *Resolver {
	return &Resolver{env: env}
}

// ConfigRoot returns the absolute path of the directory cwm keeps its
// configuration in.
//
// It is the value of CWM_CONFIG_ROOT when that variable is set to a non-blank
// value, and otherwise [AppName] under the OS user config directory
// (~/.config/cwm on Linux). The directory is not created.
func (r *Resolver) ConfigRoot() (string, error) {
	return r.root("config", ConfigRootEnv, r.env.UserConfigDir)
}

// CacheRoot returns the absolute path of the directory cwm keeps its cached
// data in.
//
// It is the value of CWM_CACHE_ROOT when that variable is set to a non-blank
// value, and otherwise [AppName] under the OS user cache directory
// (~/.cache/cwm on Linux). The directory is not created.
//
// Everything under this root must be safe to delete: cwm treats it as
// reconstructible.
func (r *Resolver) CacheRoot() (string, error) {
	return r.root("cache", CacheRootEnv, r.env.UserCacheDir)
}

// HomeDir returns the current user's home directory.
//
// Unlike the roots above this has no override: it is the OS's answer, and the
// settings derived from it are overridden in the config document instead.
func (r *Resolver) HomeDir() (string, error) {
	dir, err := r.env.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate the home directory: %w", err)
	}

	return dir, nil
}

// root returns the override named by envVar when it is set to a non-blank
// value, and otherwise [AppName] under the directory reported by base. kind
// names the root for error messages.
func (r *Resolver) root(kind, envVar string, base func() (string, error)) (string, error) {
	if override, ok := r.env.LookupEnv(envVar); ok && strings.TrimSpace(override) != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve %s=%q to an absolute path: %w", envVar, override, err)
		}

		return abs, nil
	}

	dir, err := base()
	if err != nil {
		return "", fmt.Errorf("locate the user %s directory (set %s to override): %w", kind, envVar, err)
	}

	return filepath.Join(dir, AppName), nil
}
