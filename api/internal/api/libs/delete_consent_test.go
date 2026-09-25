package libs_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
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

func TestWriteDeleteErr_ARefusalOverWorkAtRiskCarriesTheList(t *testing.T) {
	ctx, rec := newCtx(t)
	refused := &domain.WorkAtRiskError{Workspaces: []domain.WorkAtRisk{
		{WorkspaceID: "w1", Branch: "feature/x", UncommittedFiles: 2, UnmergedCommits: 1},
	}}

	libs.WriteDeleteErr(ctx, fmt.Errorf("delete cascade: %w", refused))

	assert.Equal(t, http.StatusConflict, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, libs.WorkAtRiskCode, body["code"])
	assert.Contains(t, body["error"], "feature/x (2 uncommitted files, 1 unmerged commits)")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{map[string]any{
		"workspaceId": "w1", "branch": "feature/x", "uncommittedFiles": 2.0, "unmergedCommits": 1.0,
	}}, data["workAtRisk"])
}

func TestWriteDeleteErr_AnyOtherErrorMapsAsUsual(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteDeleteErr(ctx, fmt.Errorf("x: %w", apperr.ErrNotFound))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Nil(t, decode(t, rec.Body.Bytes())["code"])
}
