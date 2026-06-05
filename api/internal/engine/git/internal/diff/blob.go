package diff

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	".bmp":  true,
}

func isImagePath(
	path string,
) bool {
	lower := strings.ToLower(path)
	for ext := range imageExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func populateBlobData(
	ctx context.Context,
	repoPath string,
	f domain.FileDiff,
	oldSHA string,
	newSHA string,
) domain.FileDiff {
	if !f.IsBinary && !f.IsImage {
		return f
	}
	if oldSHA != "" && !isZeroSHA(oldSHA) {
		data, err := fetchBlob(ctx, repoPath, oldSHA)
		if err == nil {
			f.OldBlobBase64 = data
		}
	}
	if newSHA != "" && !isZeroSHA(newSHA) {
		data, err := fetchBlob(ctx, repoPath, newSHA)
		if err == nil {
			f.NewBlobBase64 = data
		}
	}
	return f
}

func isZeroSHA(
	sha string,
) bool {
	for _, c := range sha {
		if c != '0' {
			return false
		}
	}
	return true
}

func fetchBlob(
	ctx context.Context,
	repoPath string,
	sha string,
) (string, error) {
	r, _ := exec.Git(ctx, repoPath, "show", sha)
	if err := exec.RequireSuccess("diff: fetch blob", r); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(r.Stdout)), nil
}
