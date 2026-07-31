package loopback_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/loopback"
)

const testToken = "s3cr3t-token-value"

func servedBehindAuth(
	token string,
) (http.Handler, *bool) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusTeapot)
	})
	return loopback.Authenticate(token, next), &reached
}

func TestAuthenticate_Accepts(t *testing.T) {
	cases := map[string]func(*http.Request){
		"authorization bearer": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+testToken)
		},
		"authorization bearer lowercase": func(r *http.Request) {
			r.Header.Set("Authorization", "bearer "+testToken)
		},
		"authorization bearer mixed case": func(r *http.Request) {
			r.Header.Set("Authorization", "BeArEr "+testToken)
		},
		"authorization bearer with padding": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer   "+testToken+"  ")
		},
		"crowbar header": func(r *http.Request) {
			r.Header.Set(loopback.HeaderName, testToken)
		},
		"query parameter": func(r *http.Request) {
			r.URL.RawQuery = loopback.QueryParam + "=" + testToken
		},
		"wrong header but right query": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer nope")
			r.URL.RawQuery = loopback.QueryParam + "=" + testToken
		},
	}
	for name, present := range cases {
		t.Run(name, func(t *testing.T) {
			handler, reached := servedBehindAuth(testToken)
			req := httptest.NewRequest(http.MethodGet, "/v0/health", nil)
			present(req)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusTeapot, rec.Code)
			assert.True(t, *reached, "an authenticated request must reach the wrapped handler")
		})
	}
}

func TestAuthenticate_Rejects(t *testing.T) {
	cases := map[string]func(*http.Request){
		"no credential at all": func(*http.Request) {},
		"empty bearer": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer ")
		},
		"bare token without the scheme": func(r *http.Request) {
			r.Header.Set("Authorization", testToken)
		},
		"a different auth scheme": func(r *http.Request) {
			r.Header.Set("Authorization", "Basic "+testToken)
		},
		"wrong token": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer wrong")
		},
		"token prefix only": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+testToken[:5])
		},
		"empty crowbar header": func(r *http.Request) {
			r.Header.Set(loopback.HeaderName, "")
		},
		"empty query parameter": func(r *http.Request) {
			r.URL.RawQuery = loopback.QueryParam + "="
		},
	}
	for name, present := range cases {
		t.Run(name, func(t *testing.T) {
			handler, reached := servedBehindAuth(testToken)
			req := httptest.NewRequest(http.MethodGet, "/v0/health", nil)
			present(req)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.False(t, *reached, "an unauthenticated request must not reach the wrapped handler")
			assert.Equal(t, `Bearer realm="crowbar"`, rec.Header().Get("WWW-Authenticate"))
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			assert.Contains(t, rec.Body.String(), `"success":false`)
		})
	}
}

// TestAuthenticate_EmptyExpectedToken_FailsClosed is the misconfiguration case: a
// wrapper built with no token must deny everything, including a caller presenting
// the empty string, rather than admitting everyone.
func TestAuthenticate_EmptyExpectedToken_FailsClosed(t *testing.T) {
	handler, reached := servedBehindAuth("")

	for _, presented := range []string{"", "anything"} {
		req := httptest.NewRequest(http.MethodGet, "/v0/health", nil)
		req.Header.Set(loopback.HeaderName, presented)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.False(t, *reached)
	}
}
