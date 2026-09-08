package update_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/internal/update"
)

// entry is one file to put in a test archive.
type entry struct {
	name     string
	contents string
	dir      bool
}

// makeArchive builds a gzipped tar holding entries, in order.
func makeArchive(t *testing.T, entries ...entry) []byte {
	t.Helper()

	var buffer bytes.Buffer

	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)

	for _, e := range entries {
		header := &tar.Header{
			Name:     e.name,
			Mode:     0o755,
			Size:     int64(len(e.contents)),
			Typeflag: tar.TypeReg,
		}

		if e.dir {
			header.Typeflag = tar.TypeDir
			header.Size = 0
		}

		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}

		if !e.dir {
			if _, err := archive.Write([]byte(e.contents)); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}

	if err := compressed.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}

	return buffer.Bytes()
}

// checksumsFor renders a goreleaser checksums file for the given archives.
func checksumsFor(named map[string][]byte) []byte {
	var lines strings.Builder

	for name, data := range named {
		sum := sha256.Sum256(data)
		lines.WriteString(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	}

	return []byte(lines.String())
}

func TestVerifyChecksum(t *testing.T) {
	t.Parallel()

	archive := []byte("the archive bytes")
	checksums := checksumsFor(map[string][]byte{"cwm_1.2.3_linux_amd64.tar.gz": archive})

	if err := update.VerifyChecksum(archive, checksums, "cwm_1.2.3_linux_amd64.tar.gz"); err != nil {
		t.Errorf("VerifyChecksum() error = %v, want nil", err)
	}
}

func TestVerifyChecksumRejectsATamperedArchive(t *testing.T) {
	t.Parallel()

	checksums := checksumsFor(map[string][]byte{"cwm.tar.gz": []byte("the real archive")})

	err := update.VerifyChecksum([]byte("something else entirely"), checksums, "cwm.tar.gz")
	if !errors.Is(err, update.ErrChecksumMismatch) {
		t.Errorf("VerifyChecksum() error = %v, want ErrChecksumMismatch", err)
	}
}

func TestVerifyChecksumRejectsAnUnlistedArchive(t *testing.T) {
	t.Parallel()

	// An archive nobody published a digest for is as untrustworthy as one that
	// fails the digest.
	checksums := checksumsFor(map[string][]byte{"something_else.tar.gz": []byte("x")})

	err := update.VerifyChecksum([]byte("x"), checksums, "cwm.tar.gz")
	if !errors.Is(err, update.ErrNoChecksum) {
		t.Errorf("VerifyChecksum() error = %v, want ErrNoChecksum", err)
	}
}

func TestVerifyChecksumIgnoresJunkLines(t *testing.T) {
	t.Parallel()

	archive := []byte("the archive bytes")
	sum := sha256.Sum256(archive)
	checksums := []byte("\n# a comment\nnot-a-line\n" + hex.EncodeToString(sum[:]) + "  cwm.tar.gz\n")

	if err := update.VerifyChecksum(archive, checksums, "cwm.tar.gz"); err != nil {
		t.Errorf("VerifyChecksum() error = %v, want nil", err)
	}
}

func TestExtractBinary(t *testing.T) {
	t.Parallel()

	archive := makeArchive(t,
		entry{name: "LICENSE", contents: "a licence", dir: false},
		entry{name: "README.md", contents: "some docs", dir: false},
		entry{name: "cwm", contents: "the binary", dir: false},
	)

	binary, err := update.ExtractBinary(archive)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}

	if string(binary) != "the binary" {
		t.Errorf("ExtractBinary() = %q, want %q", binary, "the binary")
	}
}

func TestExtractBinaryFindsANestedBinary(t *testing.T) {
	t.Parallel()

	archive := makeArchive(t,
		entry{name: "cwm_1.2.3_linux_amd64/", contents: "", dir: true},
		entry{name: "cwm_1.2.3_linux_amd64/cwm", contents: "the binary", dir: false},
	)

	binary, err := update.ExtractBinary(archive)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}

	if string(binary) != "the binary" {
		t.Errorf("ExtractBinary() = %q, want %q", binary, "the binary")
	}
}

func TestExtractBinaryWithoutABinary(t *testing.T) {
	t.Parallel()

	archive := makeArchive(t, entry{name: "LICENSE", contents: "a licence", dir: false})

	if _, err := update.ExtractBinary(archive); !errors.Is(err, update.ErrNoBinary) {
		t.Errorf("ExtractBinary() error = %v, want ErrNoBinary", err)
	}
}

func TestExtractBinaryRejectsRubbish(t *testing.T) {
	t.Parallel()

	if _, err := update.ExtractBinary([]byte("not a gzip stream at all")); err == nil {
		t.Error("ExtractBinary() error = nil, want an error")
	}
}

func TestReplaceExecutable(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "cwm")
	if err := os.WriteFile(target, []byte("the old and rather longer binary"), 0o755); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := update.ReplaceExecutable([]byte("new"), target); err != nil {
		t.Fatalf("ReplaceExecutable() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != "new" {
		t.Errorf("target = %q, want %q", data, "new")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0o755 {
		t.Errorf("target mode = %#o, want %#o", mode, 0o755)
	}
}

func TestReplaceExecutableLeavesNothingBehindOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "cwm")

	// The parent of the target does not exist, so the temporary file cannot be
	// created and nothing should be left in the directory that does.
	if err := update.ReplaceExecutable([]byte("new"), target); err == nil {
		t.Fatal("ReplaceExecutable() error = nil, want an error")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("directory holds %d entries, want none", len(entries))
	}
}
