package libs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func TestWriteUntrustedFileKeepsOnlyInertTypesInline(t *testing.T) {
	cases := map[string]bool{
		"image/png":                 true,
		"image/jpeg":                true,
		"video/mp4":                 true,
		"audio/mpeg":                true,
		"application/pdf":           true,
		"image/svg+xml":             false,
		"text/html; charset=utf-8":  false,
		"text/xml; charset=utf-8":   false,
		"application/javascript":    false,
		"text/plain; charset=utf-8": false,
		"application/octet-stream":  false,
		"not a media type":          false,
	}
	for contentType, inline := range cases {
		t.Run(contentType, func(t *testing.T) {
			c, rec := newCtx(t)

			libs.WriteUntrustedFile(c, contentType, []byte("body"))

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "body", rec.Body.String())
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			csp := rec.Header().Get("Content-Security-Policy")
			if inline {
				assert.Equal(t, "inline", rec.Header().Get("Content-Disposition"))
				assert.Empty(t, csp)
				return
			}
			assert.Equal(t, "attachment", rec.Header().Get("Content-Disposition"))
			assert.Contains(t, csp, "sandbox")
			assert.Contains(t, csp, "default-src 'none'")
		})
	}
}
