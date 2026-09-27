package pairing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing"
	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubPairing struct{}

func (stubPairing) Mint(time.Time) (string, error) { return "111111", nil }

func (stubPairing) Redeem(context.Context, string, string, time.Time) (string, domain.Client, error) {
	return "tok", domain.Client{ID: "c1"}, nil
}

type stubRegistry struct{}

func (stubRegistry) FindAll(context.Context) ([]domain.Client, error) { return nil, nil }

func (stubRegistry) Save(context.Context, domain.Client) error { return nil }

func (stubRegistry) FindByKey(context.Context, string) (*domain.Client, error) {
	return &domain.Client{ID: "c1"}, nil
}

// countingLimiter aborts with 429 once it has let max requests through — a
// deterministic stand-in for the real newFixedWindowLimiter (package v0,
// unreachable here without an import cycle back through router.go).
func countingLimiter(max int) gin.HandlerFunc {
	n := 0
	return func(c *gin.Context) {
		n++
		if n > max {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}

func do(t *testing.T, r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

// doPair posts a well-formed redeem body — Pair's own 400-on-malformed-JSON
// branch is handlers_test.go's concern, not this route-wiring test's.
func doPair(t *testing.T, r *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pair", strings.NewReader(`{"code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestRegisterOpen_MountsMintAndPair(t *testing.T) {
	h := pairinghandlers.New(stubPairing{}, stubRegistry{})
	r := gin.New()
	pairing.RegisterOpen(r.Group(""), h, countingLimiter(1000))

	assert.Equal(t, http.StatusOK, do(t, r, http.MethodPost, "/pair/code").Code)
	assert.Equal(t, http.StatusOK, doPair(t, r).Code)
}

// TestRegisterOpen_LimiterAppliesToPairOnly proves the redeem limiter guards
// POST /pair alone: MintCode must stay reachable no matter how many times a
// caller has hit /pair, and vice versa.
func TestRegisterOpen_LimiterAppliesToPairOnly(t *testing.T) {
	h := pairinghandlers.New(stubPairing{}, stubRegistry{})
	r := gin.New()
	pairing.RegisterOpen(r.Group(""), h, countingLimiter(2))

	require.Equal(t, http.StatusOK, doPair(t, r).Code)
	require.Equal(t, http.StatusOK, doPair(t, r).Code)
	assert.Equal(t, http.StatusTooManyRequests, doPair(t, r).Code,
		"the third redeem attempt in the window must be capped")

	for range 5 {
		assert.Equal(t, http.StatusOK, do(t, r, http.MethodPost, "/pair/code").Code,
			"mint must never be capped by the redeem-only limiter")
	}
}

func TestRegisterGuarded_MountsListAndRevoke(t *testing.T) {
	h := pairinghandlers.New(stubPairing{}, stubRegistry{})
	r := gin.New()
	pairing.RegisterGuarded(r.Group(""), h)

	assert.Equal(t, http.StatusOK, do(t, r, http.MethodGet, "/clients").Code)
	assert.Equal(t, http.StatusOK, do(t, r, http.MethodDelete, "/clients/c1").Code)
}
