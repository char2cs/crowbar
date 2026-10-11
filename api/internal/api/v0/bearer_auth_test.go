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
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubClientStore stands in for store.ScopedStore[domain.Client, string]: a
// fixed set of clients, matched on TokenHash the way requireBearerAuth calls
// FindWhere, or a fixed err to exercise the store-failure branch (mirroring
// fakeRepoReader/fakeScopeReader above).
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

// TestRequireBearerAuth_NoTLS_PassesThrough proves a plaintext connection (the
// unix socket, or a loopback dev TCP bind) is out of this middleware's scope
// entirely — no header is required.
func TestRequireBearerAuth_NoTLS_PassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil) // no TLS
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "a non-TLS request must pass through unauthenticated")
}

// TestRequireBearerAuth_TLS_NoHeader_Rejects proves a TLS request with no
// Authorization header is rejected rather than treated as anonymous.
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

// TestRequireBearerAuth_TLS_UnknownToken_Rejects proves a bearer token with no
// matching client hash is rejected.
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

// TestRequireBearerAuth_TLS_ValidToken_Passes proves a token whose hash
// matches a known, non-revoked client authenticates.
func TestRequireBearerAuth_TLS_ValidToken_Passes(t *testing.T) {
	token := "abc123"
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token)},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store, newClientConns()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "expected 200 for a valid token")
}

// TestRequireBearerAuth_TLS_RevokedToken_Rejects proves a revoked client's
// token stops authenticating immediately — a security property, since a
// revoke is meant to take effect on the client's very next request.
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

// revokeOnReadStore is the real client store with an admin's revoke landing
// immediately after requireBearerAuth's lookup — the window in which a
// read-modify-write of the client row would clobber RevokedAt.
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

// deadlineRecorder is a response writer over a connection: it records every
// read deadline set on it, the way http.ResponseController reaches a conn.
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

// TestRequireBearerAuth_LiftsTheReadDeadlineOnlyForAPairedClient proves the
// pre-auth read bound stays on every request the guard refuses, so it also
// bounds the body drain after the 401, and comes off for a verified client.
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

// sqliteClientsPairedWith is the real client store holding one client, c1,
// paired with token.
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

// TestRegression_PairedClient_EmptyHashMatchesNoClient proves an empty token
// hash authenticates no one: the store's struct match ignores a zero field,
// so handing it "" would otherwise return every paired client.
func TestRegression_PairedClient_EmptyHashMatchesNoClient(t *testing.T) {
	clients := sqliteClientsPairedWith(t, "abc123")

	got, err := pairedClient(t.Context(), clients, "")

	require.NoError(t, err)
	assert.Nil(t, got, "an empty hash must match no client")
}

// TestRegression_RequireBearerAuth_BlankTokenIsUnauthenticated proves a bearer
// header carrying no token, or only whitespace, is refused against a store
// that does hold a paired client.
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

// TestRegression_RequireBearerAuth_ConcurrentRevokeIsNotUndone proves a
// revoke that lands while an authenticated request is in flight stays
// revoked: the auth guard never writes the client row back.
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

// TestRequireBearerAuth_TLS_StoreError_Returns500 proves a client-lookup
// failure (a storage read failure, not an unknown token) is surfaced as a
// mapped 500 and aborts the chain, mirroring the reader-failure coverage
// scopeRepoToPath/scopeWorkspaceToPath already carry in this file.
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
