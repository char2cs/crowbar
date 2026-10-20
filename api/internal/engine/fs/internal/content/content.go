// Package content reads and writes file bytes; detects text vs binary (05 §3).
package content

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// maxReadBytes is the upper bound on file size for content reads. Files larger
// than this threshold are rejected with ErrFileTooLarge to prevent OOM when a
// caller requests a multi-GB file (security/hardening finding R16).
const maxReadBytes = 25 << 20 // 25 MiB

// ErrFileTooLarge is re-exported from safepath for callers that import only
// this package. The canonical definition lives in safepath so api/libs can
// import it without violating Go's internal-package rule.
var ErrFileTooLarge = safepath.ErrFileTooLarge

// Read returns the content of a file. Binary files are base64-encoded with
// Encoding set to "base64"; text files are returned as UTF-8.
func Read(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	return readWithCap(repoPath, filePath, maxReadBytes)
}

// ReadAbs reads a host file by absolute, clean path under the same cap as Read.
func ReadAbs(
	abs string,
) (domain.FileContent, error) {
	if err := requireAbs(abs); err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", abs, err)
	}
	fc, err := readFile(abs, maxReadBytes)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", abs, err)
	}
	return fc, nil
}

// readWithCap is the internal implementation of Read with an injectable cap so
// tests can exercise the size-rejection path without creating a 25 MiB file.
func readWithCap(
	repoPath string,
	filePath string,
	cap int64,
) (domain.FileContent, error) {
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	fc, err := readFile(full, cap)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	return fc, nil
}

// readFile reads full, which the caller has already confined, rejecting
// anything over cap. Stat gives a fast rejection; LimitReader is the hard cap
// in case the file grows between Stat and the read.
func readFile(
	full string,
	cap int64,
) (domain.FileContent, error) {
	info, err := os.Stat(full)
	if err != nil {
		return domain.FileContent{}, err
	}
	if info.Size() > cap {
		return domain.FileContent{}, ErrFileTooLarge
	}

	f, err := os.Open(full) //nolint:gosec // G304: full is safepath-resolved or an absolute path the file usecase resolved
	if err != nil {
		return domain.FileContent{}, err
	}
	defer f.Close() //nolint:errcheck // read-only handle: a close error cannot lose data

	// Read at most cap+1 bytes. If we get cap+1 bytes the file grew past the
	// cap between Stat and Open; reject it with ErrFileTooLarge.
	data, err := io.ReadAll(io.LimitReader(f, cap+1))
	if err != nil {
		return domain.FileContent{}, err
	}
	if int64(len(data)) > cap {
		return domain.FileContent{}, ErrFileTooLarge
	}

	if isBinary(data) {
		return domain.FileContent{
			Content:  base64.StdEncoding.EncodeToString(data),
			Encoding: "base64",
		}, nil
	}

	return domain.FileContent{Content: string(data)}, nil
}

// Write persists content to a file, creating parent directories as needed. The
// encoding mirrors the Read side: "base64" means content is a base64-encoded
// byte payload — decoded and written verbatim so binary uploads stay
// byte-faithful — while "" or "utf8" writes content as raw UTF-8 bytes.
func Write(
	repoPath string,
	filePath string,
	content string,
	encoding string,
) error {
	// decode the payload before touching the filesystem so a malformed base64
	// body fails fast without a partial write.
	data, err := decodeContent(content, encoding)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	if err := writeFile(full, data); err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	return nil
}

// WriteAbs writes a host file by absolute, clean path, exactly as Write does.
func WriteAbs(
	abs string,
	content string,
	encoding string,
) error {
	if err := requireAbs(abs); err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	data, err := decodeContent(content, encoding)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	if err := writeFile(abs, data); err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	return nil
}

// writeFile creates full's parent directories and writes data atomically.
func writeFile(
	full string,
	data []byte,
) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("mkdirall: %w", err)
	}
	return writeAtomic(full, data)
}

func requireAbs(
	abs string,
) error {
	if !filepath.IsAbs(abs) || filepath.Clean(abs) != abs {
		return safepath.ErrNotAbsolute
	}
	return nil
}

// writeAtomic writes data to full by first writing a same-directory temp file
// and then renaming it over the target. On POSIX the rename is atomic, which
// gives two data-safety guarantees os.WriteFile does not:
//
//   - A crash or error mid-write leaves the ORIGINAL file intact. os.WriteFile
//     opens the target with O_TRUNC, so it destroys the prior content before the
//     first byte lands — a daemon SIGQUIT or a full disk during a large save then
//     leaves a truncated stub with the original gone. Writing to a temp and
//     renaming means a failed write only ever discards the temp.
//   - A concurrent reader sees either the whole old file or the whole new one,
//     never a half-written middle.
//
// The temp lives in the target's directory (rename must stay on one filesystem)
// under a hidden ".<name>.tmp-*" name; it exists for sub-millisecond and is the
// same atomic-save pattern external editors already force the watcher to tolerate.
//
// Two properties of os.WriteFile are preserved so the atomic upgrade is not a
// silent regression:
//   - A symlinked target is written THROUGH (its target updated, the link kept).
//     The temp+rename would otherwise replace the link with a regular file. A
//     dangling link is refused: its target is never created (writeThrough).
//   - An existing file's permission bits are carried onto the replacement, so
//     saving an executable script does not strip its +x. os.WriteFile kept them
//     because O_TRUNC never chmods an existing file; a fresh inode would not.
func writeAtomic(
	full string,
	data []byte,
) (err error) {
	if fi, err := os.Lstat(full); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return writeThrough(full, data)
	}

	perm := os.FileMode(0o600)
	if info, err := os.Stat(full); err == nil {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(full)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(full)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// A failure leaves no temp behind; a removal that fails too is reported with it.
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpName))
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, full)
}

// writeThrough writes data into the existing file link names, never creating
// it: a dangling link can point into the daemon's state or out of the worktree.
func writeThrough(
	link string,
	data []byte,
) error {
	f, err := os.OpenFile(link, os.O_WRONLY|os.O_TRUNC, 0) //nolint:gosec // G304: link is safepath-resolved or an absolute path the file usecase resolved
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", safepath.ErrDanglingSymlink, link)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

// decodeContent turns a write payload into the exact bytes to persist. A
// "base64" encoding is the write counterpart of Read's binary base64 encoding;
// "" and "utf8" pass the string bytes through unchanged.
func decodeContent(
	content string,
	encoding string,
) ([]byte, error) {
	switch encoding {
	case "", "utf8":
		return []byte(content), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("decode base64: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported encoding %q", encoding)
	}
}

// isBinary returns true if data contains a null byte or is not valid UTF-8.
// Fix 4: added UTF-8 validity check so Latin-1 / invalid-UTF-8 files are
// base64-encoded rather than returned as corrupted string content.
//
// It scans the WHOLE buffer, not a leading sample. A leading 8 KiB sample was a
// silent-corruption bug: a file whose first 8 KiB is clean UTF-8 but which holds
// an invalid byte later (e.g. a lone 0xE9 Latin-1 'é' at offset 9000) was
// classed as text and returned as a Go string. Go's encoding/json (gin's
// marshaller) then replaces every invalid UTF-8 byte in that string with U+FFFD,
// so the client received corrupted content and a subsequent save persisted the
// corruption. The buffer is already fully in memory (capped at maxReadBytes), so
// a full scan is O(n) over ≤25 MiB — cheap, and the only way to be byte-faithful.
func isBinary(
	data []byte,
) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(data)
}
