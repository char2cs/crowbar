// Package content reads and writes file bytes; detects text vs binary (05 §3).
package content

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Read returns the content of a file. Binary files are base64-encoded with
// Encoding set to "base64"; text files are returned as UTF-8.
func Read(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	full := filepath.Join(repoPath, filePath)
	data, err := os.ReadFile(full)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
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
	full := filepath.Join(repoPath, filePath)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		return fmt.Errorf("content: mkdirall %s: %w", filePath, err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
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
