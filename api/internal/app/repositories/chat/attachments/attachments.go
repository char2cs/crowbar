package attachments

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

const MaxBytes = 25 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

var unsafeFileNameChars = regexp.MustCompile(`[\s()\[\]]+`)

func sanitizeName(name string) string {
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	return unsafeFileNameChars.ReplaceAllString(base, "_")
}

func Store(
	dir, id, originalName string,
	data []byte,
) (fileName, contentType string, err error) {
	if len(data) > MaxBytes {
		return "", "", fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
	}
	size := int64(len(data))
	if _, err := BeginUpload(dir, id, originalName, size); err != nil {
		return "", "", err
	}
	if _, err := AppendUpload(dir, id, 0, bytes.NewReader(data)); err != nil && !errors.Is(err, ErrNotFound) {
		return "", "", err
	}
	return CompleteUpload(dir, id, originalName, size)
}

func SyntheticName(contentType string, now func() string) string {
	return "pasted-image-" + now() + extensionFor(contentType)
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

func ContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(head, []byte("<svg")) {
		return "image/svg+xml"
	}
	return ct
}

var ErrNotFound = problem.New(problem.NotFound, "attachments: not found")

func Read(
	dir, fileName string,
) (data []byte, contentType string, err error) {
	if fileName == "" || fileName != filepath.Base(fileName) {
		return nil, "", ErrNotFound
	}
	path := filepath.Join(dir, fileName)
	info, statErr := os.Stat(path)
	if statErr != nil || info.Size() > MaxBytes {
		return nil, "", ErrNotFound
	}
	//nolint:gosec // G304: dir is chat-scoped and already resolved by the caller; fileName is verified above to carry no separators.
	f, err := os.Open(path)
	if err != nil {
		return nil, "", ErrNotFound
	}
	data, err = io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if closeErr := f.Close(); closeErr != nil {
		return nil, "", fmt.Errorf("attachments: close %s: %w", fileName, closeErr)
	}
	if err != nil || int64(len(data)) > MaxBytes {
		return nil, "", ErrNotFound
	}
	return data, ContentType(data), nil
}
