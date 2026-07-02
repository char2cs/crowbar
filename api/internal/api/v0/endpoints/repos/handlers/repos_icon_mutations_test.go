package handlers_test

// Regression coverage for the two avatar-flakiness root causes fixed on
// 2026-07-01:
//  1. Icon mutations saved the repo but never broadcast the updated RepoDTO on
//     the repos WS stream, so the sidebar logo never refreshed.
//  2. isSingleEmoji required a single CODE POINT, rejecting most real emoji
//     (variation selectors, ZWJ sequences, flags, skin tones) with a 400.

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// broadcastIconRouter mounts ALL icon mutation routes on a handler whose
// broadcast fan-out is captured into frames.
func broadcastIconRouter(
	store repohandlers.Store,
	home string,
	fetch repohandlers.AvatarBytesFetcher,
	frames *[]dto.RepoDTO,
) *gin.Engine {
	h := repohandlers.NewWithDeps(store, nil, nil, func(d dto.RepoDTO) {
		*frames = append(*frames, d)
	}).WithIconStorage(func() (string, error) { return home, nil }, fetch)
	r := gin.New()
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon/emoji", h.PutIconEmoji)
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon/github", h.PutIconGithub)
	r.DELETE("/v0/projects/:projectId/repos/:repoId/icon", h.DeleteIcon)
	return r
}

func TestPutIcon_BroadcastsVersionedRepoDTO(t *testing.T) {
	home := t.TempDir()
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Name: "Repo"}}
	var frames []dto.RepoDTO
	r := broadcastIconRouter(store, home, nil, &frames)

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, frames, 1, "icon upload must broadcast the updated RepoDTO on the repos WS stream")
	assert.Equal(t, "r1", frames[0].ID)
	// The broadcast URL must carry the bumped version so <img> caches bust.
	assert.Equal(t, "/v0/projects/p1/repos/r1/icon?v=1", frames[0].AvatarURL)
}

func TestPutIconGithub_BroadcastsVersionedRepoDTO(t *testing.T) {
	home := t.TempDir()
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("gh-bytes")...)
	fetch := func(context.Context, string) ([]byte, string, error) { return png, "image/png", nil }

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: t.TempDir(), AvatarVersion: 4}}
	var frames []dto.RepoDTO
	r := broadcastIconRouter(store, home, fetch, &frames)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, frames, 1, "github icon must broadcast the updated RepoDTO")
	assert.Equal(t, "/v0/projects/p1/repos/r1/icon?v=5", frames[0].AvatarURL)
}

func TestPutIconEmoji_BroadcastsRepoDTO(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	var frames []dto.RepoDTO
	r := broadcastIconRouter(store, t.TempDir(), nil, &frames)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
		strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, frames, 1, "emoji change must broadcast the updated RepoDTO")
	assert.Equal(t, "🦊", frames[0].AvatarEmoji)
	assert.Empty(t, frames[0].AvatarURL, "emoji clears the icon URL")
}

func TestDeleteIcon_BroadcastsRepoDTO(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true, AvatarEmoji: ""}}
	var frames []dto.RepoDTO
	r := broadcastIconRouter(store, t.TempDir(), nil, &frames)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, frames, 1, "icon reset must broadcast the updated RepoDTO")
	assert.Empty(t, frames[0].AvatarURL)
	assert.Empty(t, frames[0].AvatarEmoji)
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
			var frames []dto.RepoDTO
			r := broadcastIconRouter(store, t.TempDir(), nil, &frames)

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
		store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
		var frames []dto.RepoDTO
		r := broadcastIconRouter(store, t.TempDir(), nil, &frames)

		req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
			strings.NewReader(fmt.Sprintf(`{"emoji":%q}`, bad)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "input %q must be rejected", bad)
		assert.Empty(t, frames, "rejected emoji must not broadcast")
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
