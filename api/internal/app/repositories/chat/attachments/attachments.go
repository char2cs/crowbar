// Package attachments is the durable per-chat attachment store: validating,
// naming, writing and serving the files a chat's composer sends alongside a
// message — images, PDFs, and anything too large or too binary to inline in
// the message text itself (chat attachments design spec).
//
// It is the storage half of a handler -> usecase -> storage layering that
// mirrors agentactivity's own Payload seam (ReadToolPayload -> EventStore.Payload
// -> content store): chat/handlers owns the HTTP ingestion shapes, the chat
// usecase resolves WHICH directory a chat's attachments live in, and this
// package owns what happens once both bytes and a destination are known.
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

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// MaxBytes caps a single stored attachment at 25 MiB.
//
// Icons cap at 2 MiB (a small avatar) and the tool-call content store caps at
// 8 MiB (bounded provider output); a chat attachment is neither — it is a
// user-picked image, PDF or CSV export, and a modern phone photo alone can
// clear 10-15 MiB before a scanned PDF is even in the picture. 25 MiB matches
// this codebase's own existing ceiling for a single file read
// (safepath.ErrFileTooLarge, the workspace file-read cap) rather than
// inventing a new number — one large-file boundary to reason about, not two.
const MaxBytes = 25 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidID reports whether id is safe to use as the shortid half of a stored
// filename. The frontend mints these with nanoid; this is the daemon's own
// check, never a trust of the client's generator.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var unsafeFileNameChars = regexp.MustCompile(`[\s()\[\]]+`)

// sanitizeName reduces name to a safe, markdown-safe base filename: no path
// separators, no "." or "..", and no whitespace/parens/brackets — a stored
// filename becomes the trailing segment of a ![]()/[]() reference (design
// spec), so an unescaped paren or a space in the ORIGINAL name would corrupt
// the very markdown syntax storing it. Collapsed to "_", not rejected: "my
// photo (1).png" is an entirely ordinary name to refuse outright.
func sanitizeName(name string) string {
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	return unsafeFileNameChars.ReplaceAllString(base, "_")
}

// Store writes data as a new attachment under dir (a chat's AttachmentsDir),
// named "<id>-<sanitized originalName>", creating dir if needed.
//
// Refuses with apperr.ErrInvalidArgument for an invalid id or a blank
// original name (a caller with no filename — a clipboard paste — must
// synthesize one first, see SyntheticName), with safepath.ErrFileTooLarge
// over MaxBytes, and with apperr.ErrConflict when the target filename
// already exists: the id is client-generated per attachment, so a collision
// means the SAME id was submitted twice, and overwriting silently would let
// a second upload invisibly replace bytes a message may already reference.
func Store(
	dir, id, originalName string,
	data []byte,
) (fileName, contentType string, err error) {
	if !ValidID(id) {
		return "", "", fmt.Errorf("attachments: invalid id: %w", apperr.ErrInvalidArgument)
	}
	base := sanitizeName(originalName)
	if base == "" {
		return "", "", fmt.Errorf("attachments: original name required: %w", apperr.ErrInvalidArgument)
	}
	if len(data) > MaxBytes {
		return "", "", fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
	}
	fileName = id + "-" + base
	//nolint:gosec // G301: 0o700 matches RunnerDir's own perms for daemon-managed chat state.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("attachments: create directory: %w", err)
	}
	dest := filepath.Join(dir, fileName)
	//nolint:gosec // G304: dir is chat-scoped and already resolved by the caller; fileName is validated above to carry no separators.
	// O_EXCL makes this atomic: both create and exclusive-exist check happen in one syscall, preventing TOCTOU races.
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("attachments: %q already exists: %w", fileName, apperr.ErrConflict)
		}
		return "", "", fmt.Errorf("attachments: create: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return "", "", fmt.Errorf("attachments: write: %w", err)
	}
	return fileName, ContentType(data), nil
}

// SyntheticName builds a fallback filename for an attachment with no original
// name — a clipboard image paste, which carries raw bytes and a sniffed
// content type but nothing a browser or OS ever called a "filename". now is
// injected so the name is deterministic under test. Wired into the upload
// handler (Task 4) as a defensive fallback for a blank filename — the client
// should always send a real name where it can, but this closes the gap when
// it can't.
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

// ContentType sniffs a Content-Type from data, the same way
// icons.ContentType does (deliberately re-implemented here rather than
// imported: icons lives in the API layer, and this repository package must
// not depend upward on it). http.DetectContentType has no SVG signature —
// it sniffs SVG as text/* — so SVG is special-cased.
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
var ErrNotFound = errors.New("attachments: not found")

// Read returns fileName's bytes and sniffed content type from dir, or
// ErrNotFound. Stat-rejected and capped exactly like icons.Serve: dir holds
// files this daemon wrote, but a corrupted or replaced file must never cause
// an unbounded read. fileName must be a bare filename with no separators —
// the shape Store produces — so a caller forwarding an unsanitized URL path
// segment cannot escape dir.
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
	defer func() { _ = f.Close() }()
	size := info.Size()
	data = make([]byte, size)
	_, err = io.ReadFull(f, data)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return data, ContentType(data), nil
}
