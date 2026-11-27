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

const MaxBytes = 2 << 20

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
	c.Header("Cache-Control", "no-cache")
	libs.WriteUntrustedFile(c, ContentType(data), data)
}

func ReadUpload(
	c *gin.Context,
) (data []byte, ok bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return readFromPath(c)
	}
	return readFromMultipart(c)
}

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

const oversizeMessage = "icon must be under 2 MB"

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

func Store(
	path string,
	data []byte,
) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create icon directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

func Remove(
	ctx context.Context,
	path string,
) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "icons: remove the stored icon", "path", path, "err", err)
	}
}

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
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return false
	}
	return !unicode.IsLetter(r) || r > 127
}
