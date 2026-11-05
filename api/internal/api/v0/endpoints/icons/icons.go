// Package icons holds the entity-agnostic half of a sidebar icon: reading an
// upload, validating it, storing it, serving it back, and deciding what counts
// as a single emoji.
//
// Repos and projects differ only in where the bytes live, so the caller supplies
// the path and the size cap, content sniffing and SVG special case live here once.
package icons

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/rivo/uniseg"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// MaxBytes caps a stored icon at 2 MiB, so the daemon never reads an unbounded
// amount of data from a client-supplied path.
const MaxBytes = 2 << 20

// ContentType picks the Content-Type for a stored icon. http.DetectContentType
// sniffs SVG as text/*, which browsers refuse to render in an <img>, and some
// GitHub avatars are SVG, so SVG is detected explicitly as image/svg+xml.
func ContentType(
	data []byte,
) string {
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if strings.Contains(string(head), "<svg") {
		return "image/svg+xml"
	}
	return ct
}

// Serve writes the icon at path as the response, or 404s. The read is capped:
// a corrupted or replaced file must not cause an unbounded heap allocation.
func Serve(
	c *gin.Context,
	path string,
) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > MaxBytes {
		c.Status(http.StatusNotFound)
		return
	}
	//nolint:gosec // G304: path comes from the daemon's entity-scoped icon store, already stat-checked and size-capped above, not user-supplied.
	f, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer libs.CloseLogged(c.Request.Context(), "icons: close the icon", f)
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Uploads overwrite the bytes behind this URL; the DTO's ?v= param is the
	// primary cache-buster and no-cache is the backstop.
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, ContentType(data), data)
}

// ReadUpload extracts icon bytes from a JSON {"path"} body (desktop: the daemon
// reads the file) or a multipart "icon" field (web). On failure it writes the
// response and returns ok=false.
func ReadUpload(
	c *gin.Context,
) (data []byte, ok bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return readFromPath(c)
	}
	return readFromMultipart(c)
}

// readFromPath reads the icon from an absolute host path the desktop's native
// file picker chose: user-chosen, like a repo-import path, not network-supplied.
// Size is capped here and content is sniffed in Validate, never trusted by extension.
func readFromPath(
	c *gin.Context,
) ([]byte, bool) {
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "icon path required")
		return nil, false
	}
	// Stat-reject before opening: avoids an unbounded read on a huge file.
	info, err := os.Stat(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, false
	}
	if info.Size() > MaxBytes {
		libs.WriteErr(c, http.StatusBadRequest, oversizeMessage)
		return nil, false
	}
	f, err := os.Open(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, false
	}
	defer libs.CloseLogged(c.Request.Context(), "icons: close the upload", f)
	// LimitReader caps the actual read even if the file grows between Stat and Open.
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, false
	}
	if int64(len(data)) > MaxBytes {
		libs.WriteErr(c, http.StatusBadRequest, oversizeMessage)
		return nil, false
	}
	return data, true
}

// readFromMultipart reads the icon from a multipart "icon" form field. The read
// is capped at MaxBytes+1 so an oversize upload is detected without buffering
// the entire body.
func readFromMultipart(
	c *gin.Context,
) ([]byte, bool) {
	file, _, err := c.Request.FormFile("icon")
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "icon field required")
		return nil, false
	}
	defer libs.CloseLogged(c.Request.Context(), "icons: close the upload", file)

	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "read error")
		return nil, false
	}
	return data, true
}

// oversizeMessage is the 400 every size check answers with, so the two read
// paths cannot disagree about what the limit is called.
const oversizeMessage = "icon must be under 2 MB"

// Validate rejects anything that is not an image of an acceptable size, writing
// the response on failure. It sniffs content rather than trusting the extension
// or Content-Type, so a .png-named arbitrary host file is never stored.
func Validate(
	c *gin.Context,
	data []byte,
) bool {
	if len(data) > MaxBytes {
		libs.WriteErr(c, http.StatusBadRequest, oversizeMessage)
		return false
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be an image file")
		return false
	}
	return true
}

// Store writes raw icon bytes to path, creating the parent directory. The
// single icon file is content-type-agnostic (sniffed on read), so there is no
// extension to manage.
func Store(
	path string,
	data []byte,
) error {
	//nolint:gosec // G301: 0o755 is the intended perm for the daemon's own icon directory; it matches the perms it creates its project directories with.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("could not create icon directory: %w", err)
	}
	//nolint:gosec // G306: icon bytes are non-secret assets served over HTTP; 0o644 is the intended readable perm.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

// Remove deletes the stored icon at path. One already gone is not a failure; any
// other is logged, because the caller clears the record either way and Serve
// answers 404 for it from then on.
func Remove(
	ctx context.Context,
	path string,
) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "icons: remove the stored icon", "path", path, "err", err)
	}
}

// IsSingleEmoji reports whether s holds exactly one grapheme cluster that is not
// a plain ASCII letter. Graphemes, not code points, because most emoji are
// multi-codepoint sequences (variation selectors, ZWJ, flags, skin tones).
func IsSingleEmoji(
	s string,
) bool {
	if s == "" {
		return false
	}
	g := uniseg.NewGraphemes(s)
	if !g.Next() {
		return false
	}
	if g.Next() {
		return false // more than one user-perceived character
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return false
	}
	return !unicode.IsLetter(r) || r > 127
}
