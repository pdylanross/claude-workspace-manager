package update

import "context"

// VerifyChecksum exposes the digest check to the update_test package.
func VerifyChecksum(archive, checksums []byte, name string) error {
	return verifyChecksum(archive, checksums, name)
}

// ExtractBinary exposes the archive reader.
func ExtractBinary(archive []byte) ([]byte, error) {
	return extractBinary(archive)
}

// ReplaceExecutable exposes the binary swap.
func ReplaceExecutable(binary []byte, target string) error {
	return replaceExecutable(binary, target)
}

// ApplyTo exposes Apply against an explicit target, so that a test never
// overwrites the running test binary.
func ApplyTo(ctx context.Context, u *Updater, release Release, target string) (string, error) {
	return u.applyTo(ctx, release, target)
}
