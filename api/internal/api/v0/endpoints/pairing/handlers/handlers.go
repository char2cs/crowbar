// Package handlers serves the pairing and client-registry HTTP routes.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Pairing is the narrow usecase surface these handlers need.
type Pairing interface {
	Mint(now time.Time) (code string, err error)
	Redeem(ctx context.Context, code, name string, now time.Time) (token string, client domain.Client, err error)
}

// Registry is the narrow client-registry surface the list/revoke routes need.
type Registry interface {
	FindAll(ctx context.Context) ([]domain.Client, error)
	FindByKey(ctx context.Context, id string) (*domain.Client, error)
	Save(ctx context.Context, c domain.Client) error
}

// Streams ends the open streams a client holds — a WebSocket outlives the
// one request its token was checked on.
type Streams interface {
	Disconnect(clientID string)
}

// mintCodeResponse is the body of POST /pair/code. ServerFingerprint is the
// daemon's TLS identity fingerprint (tlsid.FingerprintOf's format): a human
// compares it against what the pairing client shows as the trust-on-first-
// connect check, since the code alone only proves possession of this socket,
// not which daemon a later TLS connection actually reaches.
type mintCodeResponse struct {
	Code              string `json:"code"`
	ServerFingerprint string `json:"serverFingerprint"`
}

// maxPairBodyBytes caps POST /pair's body, read before the caller holds any
// credential: a code and a short name need a few dozen bytes.
const maxPairBodyBytes = 4 << 10

// pairRequest is the body of POST /pair. Name is what the client registry
// lists the device as; max counts runes.
type pairRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"max=64"`
}

// pairResponse is the body of a successful POST /pair.
type pairResponse struct {
	Token    string `json:"token"`
	ClientID string `json:"clientId"`
}

// Handlers serves pairing and client management.
type Handlers struct {
	pairing           Pairing
	registry          Registry
	streams           Streams
	serverFingerprint string
}

// New builds the handler set. serverFingerprint is the daemon's TLS identity
// fingerprint, echoed on every minted code (see mintCodeResponse).
func New(p Pairing, registry Registry, streams Streams, serverFingerprint string) *Handlers {
	return &Handlers{pairing: p, registry: registry, streams: streams, serverFingerprint: serverFingerprint}
}

// MintCode issues a fresh pairing code. Callable only over a non-TLS
// connection — the unix socket, or a loopback dev TCP bind — which is the
// whole reason a redeem over the network can trust it (this endpoint must
// refuse a TLS caller and succeed for a plaintext one).
func (h *Handlers) MintCode(c *gin.Context) {
	if c.Request.TLS != nil {
		libs.WriteErr(c, http.StatusForbidden, "pairing codes can only be minted over the local socket")
		return
	}
	// Only a network-exposed daemon serves TLS, so only it has a fingerprint.
	if h.serverFingerprint == "" {
		libs.WriteErr(c, http.StatusConflict, "this daemon is not network-exposed, so no client can reach it "+
			"to pair: serve it on a non-loopback tcp:// address")
		return
	}
	code, err := h.pairing.Mint(time.Now())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, mintCodeResponse{Code: code, ServerFingerprint: h.serverFingerprint})
}

// Pair redeems a pairing code for a bearer token.
func (h *Handlers) Pair(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPairBodyBytes)
	var req pairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			libs.WriteErr(c, http.StatusRequestEntityTooLarge, "pairing request is too large")
			return
		}
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	token, client, err := h.pairing.Redeem(c.Request.Context(), req.Code, req.Name, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, pairing.ErrNoCode), errors.Is(err, pairing.ErrBadCode), errors.Is(err, pairing.ErrCodeExpired):
			libs.WriteErr(c, http.StatusUnauthorized, "pairing code is not valid")
		default:
			libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	libs.WriteQueryOK(c, pairResponse{Token: token, ClientID: client.ID})
}

// ListClients returns every paired client.
func (h *Handlers) ListClients(c *gin.Context) {
	out, err := h.registry.FindAll(c.Request.Context())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, out)
}

// RevokeClient permanently revokes one client and ends the streams it holds.
// No later pairing code reactivates a revoked id: a fresh pairing always
// mints a fresh client.
func (h *Handlers) RevokeClient(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.registry.FindByKey(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		libs.WriteErr(c, http.StatusNotFound, "client not found")
		return
	}
	now := time.Now()
	existing.RevokedAt = &now
	if err := h.registry.Save(c.Request.Context(), *existing); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	// After the save: an upgrade racing this re-reads the row once registered.
	h.streams.Disconnect(id)
	libs.WriteQueryOK(c, gin.H{"revoked": true})
}
