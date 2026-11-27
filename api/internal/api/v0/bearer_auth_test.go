package v0

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterstore "github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type stubClientStore struct {
	clients []domain.Client
	err     error
}

func (s *stubClientStore) FindWhere(_ context.Context, match domain.Client) ([]domain.Client, error) {
	if s.err != nil {
		return nil, s.err
	}
	var out []domain.Client
	for _, c := range s.clients {
		if match.TokenHash != "" && c.TokenHash == match.TokenHash {
			out = append(out, c)
		}
	}
	return out, nil
}

func TestRequireBearerAuth_NoTLS_PassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "a non-TLS request must pass through unauthenticated")
}

func TestRequireBearerAuth_TLS_NoHeader_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 with no Authorization header")
}

func TestRequireBearerAuth_TLS_UnknownToken_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 for an unknown token")
}

func TestRequireBearerAuth_TLS_ValidToken_Passes(t *testing.T) {
	token := "abc123"
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", Name: "phone", TokenHash: pairing.HashToken(token)},
	}}
	var who libs.PairedClient
	r := gin.New()
	r.Use(requireBearerAuth(store, newClientConns()))
	r.GET("/x", func(c *gin.Context) {
		who, _ = libs.PairedClientFrom(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "expected 200 for a valid token")
	assert.Equal(t, libs.PairedClient{ID: "c1", Name: "phone"}, who, "handlers learn which device called")
}

func TestRequireBearerAuth_TLS_RevokedToken_Rejects(t *testing.T) {
	token := "abc123"
	revokedAt := time.Now()
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token), RevokedAt: &revokedAt},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 for a revoked client's token")
}

type revokeOnReadStore struct {
	adapterstore.ScopedStore[domain.Client, string]
}

func (s revokeOnReadStore) FindWhere(ctx context.Context, match domain.Client) ([]domain.Client, error) {
	found, err := s.ScopedStore.FindWhere(ctx, match)
	for _, c := range found {
		revokedAt := time.Now()
		c.RevokedAt = &revokedAt
		if saveErr := s.Save(ctx, c); saveErr != nil {
			return nil, saveErr
		}
	}
	return found, err
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (r *deadlineRecorder) SetReadDeadline(
	deadline time.Time,
) error {
	r.deadlines = append(r.deadlines, deadline)
	return nil
}

func TestRequireBearerAuth_LiftsTheReadDeadlineOnlyForAPairedClient(t *testing.T) {
	const token = "abc123"
	store := &stubClientStore{clients: []domain.Client{{ID: "c1", TokenHash: pairing.HashToken(token)}}}
	r := gin.New()
	r.Use(requireBearerAuth(store, newClientConns()))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for bearer, lifted := range map[string]bool{token: true, "": false, "never-issued": false} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", http.NoBody)
		req.TLS = &tls.ConnectionState{}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		r.ServeHTTP(rec, req)

		if lifted {
			assert.Equalf(t, []time.Time{{}}, rec.deadlines, "bearer %q: the bound must be cleared", bearer)
			continue
		}
		assert.Emptyf(t, rec.deadlines, "bearer %q: a refused request must keep its bound", bearer)
	}
}

func sqliteClientsPairedWith(
	t *testing.T,
	token string,
) adapterstore.ScopedStore[domain.Client, string] {
	t.Helper()
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "clients.db"))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	clients, err := storesqlite.NewFromDB[domain.Client, string](db)
	require.NoError(t, err)

	pairedAt := time.Now().Add(-time.Hour)
	require.NoError(t, clients.Save(t.Context(), domain.Client{
		ID: "c1", Name: "laptop", TokenHash: pairing.HashToken(token), PairedAt: pairedAt, LastSeen: pairedAt,
	}))
	return clients
}

func TestRegression_PairedClient_EmptyHashMatchesNoClient(t *testing.T) {
	clients := sqliteClientsPairedWith(t, "abc123")

	got, err := pairedClient(t.Context(), clients, "")

	require.NoError(t, err)
	assert.Nil(t, got, "an empty hash must match no client")
}

func TestRegression_RequireBearerAuth_BlankTokenIsUnauthenticated(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(sqliteClientsPairedWith(t, "abc123"), newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, header := range []string{"Bearer ", "Bearer    ", "Bearer \t"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
		req.TLS = &tls.ConnectionState{}
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equalf(t, http.StatusUnauthorized, rec.Code, "Authorization %q", header)
	}
}

func TestRegression_RequireBearerAuth_ConcurrentRevokeIsNotUndone(t *testing.T) {
	token := "abc123"
	clients := sqliteClientsPairedWith(t, token)

	r := gin.New()
	r.Use(requireBearerAuth(revokeOnReadStore{clients}, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "the request authenticated before the revoke landed")

	got, err := clients.FindByKey(t.Context(), "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.RevokedAt, "an in-flight request must never un-revoke its client")
}

func TestRequireBearerAuth_TLS_StoreError_Returns500(t *testing.T) {
	store := &stubClientStore{err: errors.New("client store unavailable")}
	r := gin.New()
	r.Use(requireBearerAuth(store, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer abc123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code,
		"a client-store failure must abort with 500, not fall through to 401 or reach the handler")
}
