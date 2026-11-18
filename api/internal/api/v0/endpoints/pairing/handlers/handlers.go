// Package handlers serves the pairing and client-registry HTTP routes.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/exposure"
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

// Access is the device listener the codes it mints are redeemed over.
type Access interface {
	Status() exposure.Status
}

// mintCodeResponse is the body of POST /pair/code. A human compares
// ServerFingerprint (tlsid.FingerprintOf format) with the pairing client's, since
// the code proves access to this daemon but not which one a later TLS connection reaches.
// Addresses are where the device can dial; ExpiresAt is when the code stops working.
type mintCodeResponse struct {
	Code              string   `json:"code"`
	ServerFingerprint string   `json:"serverFingerprint"`
	Addresses         []string `json:"addresses"`
	ExpiresAt         string   `json:"expiresAt"`
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
	pairing  Pairing
	registry Registry
	streams  Streams
	access   Access
}

// New builds the handler set. access supplies the daemon's TLS identity
// fingerprint and addresses, echoed on every minted code (see mintCodeResponse).
func New(p Pairing, registry Registry, streams Streams, access Access) *Handlers {
	return &Handlers{pairing: p, registry: registry, streams: streams, access: access}
}

// MintCode issues a fresh pairing code. It is a guarded route: the local socket
// is trusted by its permissions and a TLS caller by its bearer, so only an
// already-trusted caller can let another device in.
func (h *Handlers) MintCode(c *gin.Context) {
	st := h.access.Status()
	// A closed listener leaves no way to redeem the code and no address to show.
	if !st.Enabled {
		libs.WriteErr(c, http.StatusConflict, "device connections are off, so no device can reach this "+
			"daemon to pair: turn on Allow device connections in Settings, or serve it on a non-loopback tcp:// address")
		return
	}
	now := time.Now()
	code, err := h.pairing.Mint(now)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	if by, remote := libs.PairedClientFrom(c); remote {
		slog.Info("pairing: code minted over TLS", "component", "pairing", "client", by.ID, "name", by.Name)
	}
	libs.WriteQueryOK(c, mintCodeResponse{
		Code: code, ServerFingerprint: st.Fingerprint, Addresses: st.Reachable,
		ExpiresAt: now.Add(pairing.CodeTTL).UTC().Format(time.RFC3339),
	})
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
