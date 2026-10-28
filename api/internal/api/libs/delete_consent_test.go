package libs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestDeleteConsentOf_OnlyAnExplicitTrueDiscards(t *testing.T) {
	for value, want := range map[string]domain.DeleteConsent{
		"":      domain.KeepWorkAtRisk,
		"1":     domain.KeepWorkAtRisk,
		"false": domain.KeepWorkAtRisk,
		"true":  domain.DiscardWorkAtRisk,
	} {
		ctx, _ := newCtx(t)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/", http.NoBody)
		if value != "" {
			ctx.Request.Header.Set(libs.DiscardWorkHeader, value)
		}
		assert.Equal(t, want, libs.DeleteConsentOf(ctx), "header %q", value)
	}
}
