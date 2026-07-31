package loopback

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "bearer "

const unauthorizedEnvelope = `{"success":false,"error":"unauthorized: this listener requires the crowbar loopback token"}`

// Authenticate wraps next so that EVERY request arriving on the loopback TCP
// listener must present token — REST, WebSocket upgrades and static assets
// alike. There is no exempt path: an unauthenticated caller cannot even learn
// the daemon's version, and a WebSocket upgrade without the token is answered
// 401 before the hijack, so the socket is never established.
//
// A caller may present the token three ways, all equivalent:
//
//	Authorization: Bearer <token>   the native client and any fetch()
//	X-Crowbar-Token: <token>        for a caller whose stack reserves Authorization
//	?crowbar_token=<token>          the browser WebSocket handshake, which cannot
//	                                set a header
//
// This wrapper is applied to the TCP listener ONLY. The unix socket keeps the
// behaviour it has always had — no credential — because its access control is
// the socket file's 0600 mode, and adding a requirement there would break every
// client shipping today.
//
// Comparison is constant-time. An empty expected token denies everything rather
// than admitting everyone, so a misconfiguration fails closed.
func Authenticate(
	token string,
	next http.Handler,
) http.Handler {
	return &authenticator{expected: []byte(token), next: next}
}

type authenticator struct {
	expected []byte
	next     http.Handler
}

func (a *authenticator) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !a.authorized(r) {
		a.deny(w)
		return
	}
	a.next.ServeHTTP(w, r)
}

func (a *authenticator) authorized(
	r *http.Request,
) bool {
	if len(a.expected) == 0 {
		return false
	}
	for _, presented := range presented(r) {
		if subtle.ConstantTimeCompare(a.expected, []byte(presented)) == 1 {
			return true
		}
	}
	return false
}

func (a *authenticator) deny(
	w http.ResponseWriter,
) {
	header := w.Header()
	header.Set("WWW-Authenticate", `Bearer realm="crowbar"`)
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(unauthorizedEnvelope))
}

func presented(
	r *http.Request,
) []string {
	candidates := make([]string, 0, 3)
	if authorization := r.Header.Get("Authorization"); len(authorization) > len(bearerPrefix) &&
		strings.EqualFold(authorization[:len(bearerPrefix)], bearerPrefix) {
		candidates = append(candidates, strings.TrimSpace(authorization[len(bearerPrefix):]))
	}
	if header := strings.TrimSpace(r.Header.Get(HeaderName)); header != "" {
		candidates = append(candidates, header)
	}
	if query := r.URL.Query().Get(QueryParam); query != "" {
		candidates = append(candidates, query)
	}
	return candidates
}
