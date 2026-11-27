package diff

import (
	"crypto/sha256"
	"fmt"
)

func HunkID(
	filePath string,
	hunkBody string,
) string {
	h := sha256.New()
	h.Write([]byte(filePath))
	h.Write([]byte("\n"))
	h.Write([]byte(hunkBody))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}
