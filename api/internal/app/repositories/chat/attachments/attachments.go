// Package attachments stores the files a chat's composer sends with a message:
// it validates, names, writes and serves them, once the chat usecase has
// resolved which directory a chat's attachments live in.
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

// MaxBytes caps a single stored attachment at 25 MiB: a phone photo or scanned PDF
// can clear 10-15 MiB, and 25 MiB matches the workspace file-read cap
// (safepath.ErrFileTooLarge), so there is one large-file boundary, not two.
const MaxBytes = 25 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidID reports whether id is safe to use as the shortid half of a stored
// filename. The frontend mints these with nanoid; this is the daemon's own
// check, never a trust of the client's generator.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var unsafeFileNameChars = regexp.MustCompile(`[\s()\[\]]+`)

// sanitizeName reduces name to a base filename with no separators, "." or "..",
// and whitespace/parens/brackets collapsed to "_": the name ends up inside a
// markdown ![]()/[]() link, and "my photo (1).png" is too ordinary to refuse.
func sanitizeName(name string) string {
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	return unsafeFileNameChars.ReplaceAllString(base, "_")
}

// Store writes data under dir as "<id>-<sanitized originalName>" through the staged upload, so it
// is never seen half-written, and never replaces an attachment a message may already reference.
// Blank names must come from SyntheticName first.
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

// SyntheticName builds a fallback filename for an attachment with no original
// name, such as a clipboard image paste. now is injected for determinism.
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

// ContentType sniffs a Content-Type from data like icons.ContentType, duplicated
// because a repository must not import the API layer. http.DetectContentType
// reports SVG as text/*, so SVG is special-cased.
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

// ErrNotFound reports that no attachment exists at the requested reference.
var ErrNotFound = problem.New(problem.NotFound, "attachments: not found")

// Read returns fileName's bytes and sniffed content type from dir, or ErrNotFound.
// It is size-capped so a replaced file cannot cause an unbounded read, and
// fileName must be a bare name so an unsanitized URL segment cannot escape dir.
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
