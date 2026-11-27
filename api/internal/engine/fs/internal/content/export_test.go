package content

import "github.com/char2cs/crowbar/api/internal/domain"

func ReadWithCap(repoPath, filePath string, cap int64) (domain.FileContent, error) {
	return readWithCap(repoPath, filePath, cap)
}
