// Package diff parses unified diff output from git into domain types.
package diff

import (
	"crypto/sha256"
	"fmt"
)

// HunkID computes a stable body-only hash for a diff hunk.
// Input is the filePath and hunkBody (the +/-/space lines, NOT the @@ header).
// Returns the first 12 hex chars of sha256(filePath + "\n" + hunkBody).
// The hash deliberately excludes the @@ header so IDs remain stable when
// sibling hunks in the same file are staged independently.
func HunkID(
	filePath string,
	hunkBody string,
) string {
	h := sha256.New()
	// Writing to a hash.Hash never returns an error.
	_, _ = fmt.Fprintf(h, "%s\n%s", filePath, hunkBody)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}
