// Package content reads and writes file bytes; detects text vs binary (05 §3).
package content

import (
	"encoding/base64"
	"fmt"
	"io"
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

	// Stat first for a fast rejection; io.LimitReader below provides the
	// hard cap in case the file grows between Stat and the actual read.
	info, err := os.Stat(full)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	if info.Size() > cap {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, ErrFileTooLarge)
	}

	// full is the output of safepath.Resolve, which confines the path to
	// repoPath and rejects traversal, so it is not attacker-controllable.
	f, err := os.Open(full) //nolint:gosec // G304: full is validated by safepath.Resolve (confined to repoPath)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	defer f.Close() //nolint:errcheck

	// Read at most cap+1 bytes. If we get cap+1 bytes the file grew past the
	// cap between Stat and Open; reject it with ErrFileTooLarge.
	data, err := io.ReadAll(io.LimitReader(f, cap+1))
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	if int64(len(data)) > cap {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, ErrFileTooLarge)
	}

	if isBinary(data) {
		return domain.FileContent{
			Content:  base64.StdEncoding.EncodeToString(data),
			Encoding: "base64",
		}, nil
	}

	return domain.FileContent{Content: string(data)}, nil
}

// Write writes UTF-8 content to a file, creating parent directories as needed.
func Write(
	repoPath string,
	filePath string,
	content string,
) error {
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("content: mkdirall %s: %w", filePath, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	return nil
}

// isBinary returns true if data contains a null byte or is not valid UTF-8.
// Fix 4: added UTF-8 validity check so Latin-1 / invalid-UTF-8 files are
// base64-encoded rather than returned as corrupted string content.
// Checks only the first 8 KiB for performance.
func isBinary(
	data []byte,
) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	sample := data[:limit]
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(sample)
}
