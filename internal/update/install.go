package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// binaryName is the file inside a release archive that cwm replaces itself
// with.
const binaryName = "cwm"

// binaryPerm is the mode the installed binary gets. It has to be executable,
// and there is no reason for it to be writable by anyone else.
const binaryPerm fs.FileMode = 0o755

// maxBinaryBytes caps what will be extracted from an archive, so that a
// malformed or hostile one cannot fill the disk.
const maxBinaryBytes = 256 << 20

// ErrChecksumMismatch is returned when a downloaded archive does not match the
// digest published with it. cwm is about to execute this file, so a mismatch is
// fatal rather than a warning.
var ErrChecksumMismatch = errors.New("downloaded archive does not match its published checksum")

// ErrNoChecksum is returned when the checksums file has no entry for the
// archive. An unverifiable download is treated the same as a bad one.
var ErrNoChecksum = errors.New("no published checksum for this archive")

// ErrNoBinary is returned when the archive does not contain a cwm binary.
var ErrNoBinary = errors.New("release archive contains no cwm binary")

// verifyChecksum reports whether archive matches its entry in a goreleaser
// checksums file, whose lines are "<hex digest>  <file name>".
func verifyChecksum(archive []byte, checksums []byte, name string) error {
	want, found := checksumFor(checksums, name)
	if !found {
		return fmt.Errorf("%w: %s", ErrNoChecksum, name)
	}

	sum := sha256.Sum256(archive)

	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: %s is %s, published as %s", ErrChecksumMismatch, name, got, want)
	}

	return nil
}

// checksumFor finds the digest published for name.
func checksumFor(checksums []byte, name string) (string, bool) {
	for line := range strings.Lines(string(checksums)) {
		digest, file, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}

		if strings.TrimSpace(file) == name {
			return digest, true
		}
	}

	return "", false
}

// extractBinary returns the cwm binary from a gzipped tar archive.
func extractBinary(archive []byte) ([]byte, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read the archive: %w", err)
	}

	defer func() { _ = compressed.Close() }()

	entries := tar.NewReader(compressed)

	for {
		header, nextErr := entries.Next()
		if errors.Is(nextErr, io.EOF) {
			return nil, ErrNoBinary
		}

		if nextErr != nil {
			return nil, fmt.Errorf("read the archive: %w", nextErr)
		}

		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}

		// Bounded so that a lying header cannot make this read forever.
		binary, readErr := io.ReadAll(io.LimitReader(entries, maxBinaryBytes))
		if readErr != nil {
			return nil, fmt.Errorf("read %s from the archive: %w", binaryName, readErr)
		}

		return binary, nil
	}
}

// replaceExecutable writes binary over the file at target.
//
// The new binary goes to a temporary file in the same directory and is renamed
// into place, which is both atomic and the only way to replace a program that
// is currently running: the old inode stays alive for this process while the
// name points at the new one.
func replaceExecutable(binary []byte, target string) error {
	dir := filepath.Dir(target)

	temp, err := os.CreateTemp(dir, "."+binaryName+"-update-*")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}

	name := temp.Name()

	defer func() { _ = os.Remove(name) }()

	if _, writeErr := temp.Write(binary); writeErr != nil {
		_ = temp.Close()

		return fmt.Errorf("write %s: %w", name, writeErr)
	}

	if closeErr := temp.Close(); closeErr != nil {
		return fmt.Errorf("close %s: %w", name, closeErr)
	}

	if chmodErr := os.Chmod(name, binaryPerm); chmodErr != nil {
		return fmt.Errorf("make %s executable: %w", name, chmodErr)
	}

	if renameErr := os.Rename(name, target); renameErr != nil {
		return fmt.Errorf("replace %s: %w", target, renameErr)
	}

	return nil
}

// executablePath returns the path of the running binary with any symlinks
// resolved, so that an update replaces the real file rather than a link to it.
func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the running cwm: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	return resolved, nil
}
