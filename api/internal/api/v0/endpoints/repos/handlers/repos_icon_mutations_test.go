package handlers_test

// Icon mutations save the repo row the repos stream publishes from, and an
// emoji is one grapheme cluster, not one code point.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// iconMutationRouter mounts ALL icon mutation routes on one handler.
func iconMutationRouter(
	store repohandlers.Store,
	home string,
	fetch repohandlers.AvatarBytesFetcher,
) *gin.Engine {
	h := repohandlers.NewWithDeps(store, nil, nil, nil, func() (string, error) { return home, nil }, nil).
		WithAvatarFetcher(fetch)
	r := gin.New()
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon/emoji", h.PutIconEmoji)
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon/github", h.PutIconGithub)
	r.DELETE("/v0/projects/:projectId/repos/:repoId/icon", h.DeleteIcon)
	return r
}

// savingStore serves repo and records the last row saved, nil until a save.
func savingStore(repo domain.Repository) (*fakeStore, **domain.Repository) {
	var saved *domain.Repository
	store := &fakeStore{byKey: &repo}
	store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = &r; return nil }
	return store, &saved
}

// The saved version is bumped so the published icon URL busts <img> caches.
func TestPutIcon_SavesABumpedVersion(t *testing.T) {
	home := t.TempDir()
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	store, saved := savingStore(domain.Repository{ID: "r1", ProjectID: "p1", Name: "Repo"})
	r := iconMutationRouter(store, home, nil)

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, *saved)
	assert.True(t, (*saved).AvatarHasIcon)
	assert.Equal(t, int64(1), (*saved).AvatarVersion)
}

func TestPutIconGithub_SavesABumpedVersion(t *testing.T) {
	home := t.TempDir()
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("gh-bytes")...)
	fetch := func(context.Context, string) ([]byte, string, error) { return png, "image/png", nil }

	store, saved := savingStore(domain.Repository{ID: "r1", ProjectID: "p1", Path: t.TempDir(), AvatarVersion: 4})
	r := iconMutationRouter(store, home, fetch)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, *saved)
	assert.True(t, (*saved).AvatarHasIcon)
	assert.Equal(t, int64(5), (*saved).AvatarVersion)
}

func TestPutIconEmoji_SavesTheEmojiOverTheIcon(t *testing.T) {
	store, saved := savingStore(domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true})
	r := iconMutationRouter(store, t.TempDir(), nil)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
		strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, *saved)
	assert.Equal(t, "🦊", (*saved).AvatarEmoji)
	assert.False(t, (*saved).AvatarHasIcon, "emoji clears the icon")
}

func TestDeleteIcon_SavesTheGeneratedAvatar(t *testing.T) {
	store, saved := savingStore(domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true, AvatarEmoji: "🦊"})
	r := iconMutationRouter(store, t.TempDir(), nil)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, *saved)
	assert.False(t, (*saved).AvatarHasIcon)
	assert.Empty(t, (*saved).AvatarEmoji)
}

// Real-world emoji are mostly multi-codepoint grapheme clusters; every one of
// these previously failed the single-code-point check with a 400.
func TestPutIconEmoji_MultiCodepointEmoji_Accepted(t *testing.T) {
	cases := map[string]string{
		"variation selector": "❤️",
		"zwj sequence":       "👨‍💻",
		"flag":               "🇦🇷",
		"skin tone":          "👍🏽",
		"keycap":             "1️⃣",
	}
	for name, emoji := range cases {
		t.Run(name, func(t *testing.T) {
			var saved domain.Repository
			store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
			store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = r; return nil }
			r := iconMutationRouter(store, t.TempDir(), nil)

			req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
				strings.NewReader(fmt.Sprintf(`{"emoji":%q}`, emoji)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNoContent, rec.Code, "emoji %q must be accepted", emoji)
			assert.Equal(t, emoji, saved.AvatarEmoji)
		})
	}
}

// Multi-grapheme strings must still be rejected — one user-perceived character.
func TestPutIconEmoji_MultiGrapheme_Rejected(t *testing.T) {
	for _, bad := range []string{"🦊🦊", "a❤️", "ab"} {
		store, saved := savingStore(domain.Repository{ID: "r1", ProjectID: "p1"})
		r := iconMutationRouter(store, t.TempDir(), nil)

		req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
			strings.NewReader(fmt.Sprintf(`{"emoji":%q}`, bad)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "input %q must be rejected", bad)
		assert.Nil(t, *saved, "a rejected emoji saves nothing")
	}
}

func TestIcon_SetsNoCacheHeader(t *testing.T) {
	home := t.TempDir()
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("x")...)
	require.NoError(t, os.WriteFile(iconPath, png, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, "/v0/projects/:projectId/repos/:repoId/icon")

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

// unusedHome is the resolver for handlers whose test never reaches an icon
// route: reaching one fails loudly instead of touching a real home.
func unusedHome() (string, error) { return "", errors.New("no crowbar home") }
