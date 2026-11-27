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

type Pairing interface {
	Mint(now time.Time) (code string, err error)
	Redeem(ctx context.Context, code, name string, now time.Time) (token string, client domain.Client, err error)
}

type Registry interface {
	FindAll(ctx context.Context) ([]domain.Client, error)
	FindByKey(ctx context.Context, id string) (*domain.Client, error)
	Save(ctx context.Context, c domain.Client) error
}

type Streams interface {
	Disconnect(clientID string)
}

type Access interface {
	Status() exposure.Status
}

type mintCodeResponse struct {
	Code              string   `json:"code"`
	ServerFingerprint string   `json:"serverFingerprint"`
	Addresses         []string `json:"addresses"`
	ExpiresAt         string   `json:"expiresAt"`
}

const maxPairBodyBytes = 4 << 10

type pairRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"max=64"`
}

type pairResponse struct {
	Token    string `json:"token"`
	ClientID string `json:"clientId"`
}

type Handlers struct {
	pairing  Pairing
	registry Registry
	streams  Streams
	access   Access
}

func New(p Pairing, registry Registry, streams Streams, access Access) *Handlers {
	return &Handlers{pairing: p, registry: registry, streams: streams, access: access}
}

func (h *Handlers) MintCode(c *gin.Context) {
	st := h.access.Status()
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

func (h *Handlers) ListClients(c *gin.Context) {
	out, err := h.registry.FindAll(c.Request.Context())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, out)
}

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
	h.streams.Disconnect(id)
	libs.WriteQueryOK(c, gin.H{"revoked": true})
}
