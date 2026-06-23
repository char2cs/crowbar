// Package content — exported shims for white-box testing only.
// This file is compiled only during `go test`.
package content

import "github.com/char2cs/crowbar/api/internal/domain"

// ReadWithCap exposes the internal readWithCap function so tests can inject a
// small byte cap without having to create a 25 MiB file.
func ReadWithCap(repoPath, filePath string, cap int64) (domain.FileContent, error) {
	return readWithCap(repoPath, filePath, cap)
}
