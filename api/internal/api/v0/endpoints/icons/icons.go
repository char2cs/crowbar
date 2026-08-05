// Package icons holds the entity-agnostic half of a sidebar icon: reading an
// upload, validating it, storing it, serving it back, and deciding what counts
// as a single emoji.
//
// Repos have had an editable icon since the sidebar redesign; projects now have
// the same one. The only thing that differs between them is WHERE the bytes
// live — <crowbarHome>/projects/<projectID>/<repoID>/icon for a repo,
// <crowbarHome>/projects/<projectID>/icon for a project — so the path is the
// caller's to supply and everything else lives here once. Duplicating it per
// entity is how the two would drift on the parts that matter: the size cap, the
// content sniffing that stops a JSON path pointing at /etc/passwd being stored
// as an image, and the SVG special case.
package icons

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/rivo/uniseg"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// MaxBytes caps a stored icon at 2 MiB. Any file larger than this is rejected
// before or immediately after opening, so the daemon never reads an unbounded
// amount of data from a client-supplied path.
const MaxBytes = 2 << 20

// ContentType picks the Content-Type for a stored icon.
//
// http.DetectContentType has no SVG signature — it sniffs SVG as text/* — and
// browsers refuse to render an <img> whose SVG is served as text/*. Some GitHub
// owner avatars are SVG (org avatars, for one), so detect SVG explicitly and
// serve image/svg+xml; otherwise the fetched icon silently degrades to the
// generated placeholder. Real raster images keep their sniffed image/* type.
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

// DefaultCrowbarHome returns the root for all crowbar-managed state: the
// CROWBAR_HOME env override when set (dev instances point it inside the
// workspace being developed), otherwise ~/.crowbar.
func DefaultCrowbarHome() (string, error) {
	if override := os.Getenv(metadata.HomeEnvVar); override != "" {
		return override, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crowbar home: %w", err)
	}
	return filepath.Join(h, ".crowbar"), nil
}

// Serve writes the icon at path as the response, or 404s.
//
// The read is stat-rejected and capped: these bytes are written by this daemon,
// but a corrupted or replaced file must not cause an unbounded heap allocation.
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
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// no-cache: revalidate on every use. The bytes change in place behind this
	// URL (uploads overwrite the same file); the ?v= param on the DTO URL is the
	// primary cache-buster, this header is the belt-and-braces layer.
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, ContentType(data), data)
}

// ReadUpload extracts icon bytes from the request, dispatching on Content-Type:
// a JSON {"path"} body (desktop — the daemon reads the file) or a multipart
// "icon" field (web). On any failure it writes the response and returns
// ok=false.
func ReadUpload(
	c *gin.Context,
) (data []byte, ok bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return readFromPath(c)
	}
	return readFromMultipart(c)
}

// readFromPath reads the icon from an absolute path supplied as JSON.
//
// Residual trust assumption: the path is an absolute host path supplied by the
// desktop client (native file-picker dialog). The daemon and the WKWebView run
// on the same host, so this is equivalent to the repo-import path trust model:
// the path is user-chosen, not attacker-controlled from the network. This path
// should eventually be replaced by a byte-upload (multipart) so the daemon never
// reads arbitrary host paths at client direction.
//
// Hardening applied:
//   - Stat-reject: file must exist and be ≤ MaxBytes before any read.
//   - LimitReader: read at most MaxBytes+1 so an oversize file is detected.
//   - Content sniffing (in Validate): content-type is derived from the first 512
//     bytes, not from the file extension, so /etc/passwd styled as photo.png is
//     rejected.
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
	defer func() { _ = f.Close() }()
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
	defer func() { _ = file.Close() }()

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

// Validate rejects anything that is not an image of an acceptable size. On
// failure it writes the response and returns false.
//
// ALWAYS by content sniffing, never by trusting the extension or the
// caller-supplied Content-Type: otherwise a non-image file could be stored by
// supplying a .png filename, or a JSON path to an arbitrary host file.
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

// IsSingleEmoji reports whether s holds exactly one user-perceived character
// (grapheme cluster) that is not a plain ASCII letter.
//
// Grapheme clusters — not code points — are the unit that matters: most real
// emoji are multi-codepoint sequences (❤️ carries a variation selector, 👨‍💻 is a
// ZWJ sequence, 🇦🇷 is a two-codepoint flag, 👍🏽 carries a skin-tone modifier)
// and must all be accepted as "a single emoji".
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
