// Package diff parses unified diff output from git into domain types.
package diff

import (
	"crypto/sha256"
	"fmt"
)

// HunkID returns the first 12 hex chars of sha256(filePath + "\n" + hunkBody). It hashes
// the body only, not the @@ header, so IDs stay stable as sibling hunks are staged.
func HunkID(
	filePath string,
	hunkBody string,
) string {
	h := sha256.New()
	// Writing to a hash.Hash never returns an error.
	h.Write([]byte(filePath))
	h.Write([]byte("\n"))
	h.Write([]byte(hunkBody))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}
