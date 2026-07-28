package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// perfEnvelope mirrors the {success, data} query envelope every v0 read answers
// with, narrowed to the perf payload.
type perfEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Enabled bool          `json:"enabled"`
		Samples []perf.Sample `json:"samples"`
	} `json:"data"`
}

func perfRouter() *gin.Engine {
	r := gin.New()
	h := systemhandlers.NewPerfHandler()
	r.GET("/v0/system/perf", h.Get)
	r.POST("/v0/system/perf", h.Set)
	return r
}

func doPerf(
	method string,
	target string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	perfRouter().ServeHTTP(rec, httptest.NewRequest(method, target, http.NoBody))
	return rec
}

// TestPerfGet_ReturnsSamples is the whole point of the endpoint: a capture run
// reads the daemon's ring from OUTSIDE the daemon, so the samples must survive
// the JSON envelope intact.
func TestPerfGet_ReturnsSamples(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})
	perf.Record("git.diff", 12*time.Millisecond)

	rec := doPerf(http.MethodGet, "/v0/system/perf")

	require.Equal(t, http.StatusOK, rec.Code)
	var got perfEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.True(t, got.Data.Enabled)
	require.Len(t, got.Data.Samples, 1)
	assert.Equal(t, "git.diff", got.Data.Samples[0].Name)
	assert.InDelta(t, 12.0, got.Data.Samples[0].DurationMS, 0.001)
}

// TestPerfGet_EmptyRingSerialisesAsArray pins the wire shape a reader parses:
// an empty ring must be [], not null, so the capture tooling can index it
// without a nil check.
func TestPerfGet_EmptyRingSerialisesAsArray(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(false)

	rec := doPerf(http.MethodGet, "/v0/system/perf")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"samples":[]`)
}

// TestPerfSet_ArmsAndClears proves arming starts a CLEAN window: a sample left
// over from a previous scenario would otherwise be attributed to this one.
func TestPerfSet_ArmsAndClears(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(false)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})
	perf.SetEnabled(true)
	perf.Record("stale.sample", time.Millisecond)
	perf.SetEnabled(false)

	rec := doPerf(http.MethodPost, "/v0/system/perf?enabled=true")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, perf.Enabled())
	assert.Empty(t, perf.Snapshot())
}

func TestPerfSet_Disarms(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	rec := doPerf(http.MethodPost, "/v0/system/perf?enabled=false")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, perf.Enabled())
}

// TestPerfSet_RejectsUnparseableEnabled guards the capture run's worst failure
// mode: a typo'd query value silently disarming the ring, so the operator
// drives a whole scenario and reads back nothing with no error to explain it.
func TestPerfSet_RejectsUnparseableEnabled(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	for _, target := range []string{
		"/v0/system/perf",
		"/v0/system/perf?enabled=",
		"/v0/system/perf?enabled=yes-please",
	} {
		rec := doPerf(http.MethodPost, target)

		assert.Equalf(t, http.StatusBadRequest, rec.Code, "target %s", target)
		assert.Truef(t, perf.Enabled(), "a rejected request must not change the arming state: %s", target)
	}
}
