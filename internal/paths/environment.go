package paths

import (
	"fmt"
	"os"
)

// OSEnvironment is the [Environment] backed by the running process: it reads
// the real environment variables and the real OS directories.
//
// The zero value is ready to use.
type OSEnvironment struct{}

// NewOSEnvironment returns the [Environment] backed by the running process.
func NewOSEnvironment() OSEnvironment {
	return OSEnvironment{}
}

// LookupEnv reports the value of an environment variable. See [os.LookupEnv].
func (OSEnvironment) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// UserConfigDir reports the OS user config directory. See [os.UserConfigDir].
func (OSEnvironment) UserConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("look up the user config directory: %w", err)
	}

	return dir, nil
}

// UserCacheDir reports the OS user cache directory. See [os.UserCacheDir].
func (OSEnvironment) UserCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("look up the user cache directory: %w", err)
	}

	return dir, nil
}

// UserHomeDir reports the current user's home directory. See [os.UserHomeDir].
func (OSEnvironment) UserHomeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("look up the home directory: %w", err)
	}

	return dir, nil
}
