package v0

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type clientAuthStore interface {
	FindWhere(ctx context.Context, match domain.Client) ([]domain.Client, error)
}

func requireBearerAuth(
	clients clientAuthStore,
	conns *clientConns,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.TLS == nil {
			c.Next()
			return
		}
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		token = strings.TrimSpace(token)
		if !ok || token == "" {
			libs.WriteErr(c, http.StatusUnauthorized, "client is not paired")
			c.Abort()
			return
		}
		tokenHash := pairing.HashToken(token)
		client, err := pairedClient(c.Request.Context(), clients, tokenHash)
		if err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "client lookup failed")
			c.Abort()
			return
		}
		if client == nil {
			libs.WriteErr(c, http.StatusUnauthorized, "client is not paired")
			c.Abort()
			return
		}
		if err := middleware.LiftReadDeadline(c); err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "connection unusable")
			c.Abort()
			return
		}
		libs.SetPairedClient(c, libs.PairedClient{ID: client.ID, Name: client.Name})
		c.Writer = &revocableWriter{
			ResponseWriter: c.Writer, ctx: c.Request.Context(),
			clients: clients, conns: conns, clientID: client.ID, tokenHash: tokenHash,
		}
		c.Next()
	}
}

func pairedClient(
	ctx context.Context,
	clients clientAuthStore,
	tokenHash string,
) (*domain.Client, error) {
	if tokenHash == "" {
		return nil, nil
	}
	matches, err := clients.FindWhere(ctx, domain.Client{TokenHash: tokenHash})
	if err != nil {
		return nil, fmt.Errorf("v0: find client by token: %w", err)
	}
	if len(matches) == 0 || matches[0].RevokedAt != nil {
		return nil, nil
	}
	return &matches[0], nil
}
