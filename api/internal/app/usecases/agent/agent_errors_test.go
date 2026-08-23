package agent_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

func TestPromptErrorCode_NamesEveryPromptFailure(t *testing.T) {
	testCases := []struct {
		err  error
		want string
	}{
		{agentusecase.ErrPromptBusy, agentusecase.PromptCodeBusy},
		{agentusecase.ErrPromptOutcomeUnknown, agentusecase.PromptCodeOutcomeUnknown},
		{agentusecase.ErrPromptAlreadyAccepted, agentusecase.PromptCodeAlreadyAccepted},
		{agentusecase.ErrPromptRequestIDConflict, agentusecase.PromptCodeRequestIDConflict},
		{agentusecase.ErrPromptUnsupported, agentusecase.PromptCodeUnsupported},
		{agentusecase.ErrPromptSessionUnavailable, agentusecase.PromptCodeSessionRequired},
	}
	seen := map[string]struct{}{}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, agentusecase.PromptErrorCode(tc.err))

			assert.Equal(t, tc.want,
				agentusecase.PromptErrorCode(fmt.Errorf("agent: submit prompt: %w", tc.err)))
		})
		_, dup := seen[tc.want]
		assert.False(t, dup, "codes must be distinct: %s", tc.want)
		seen[tc.want] = struct{}{}
	}
}

func TestPromptErrorCode_HasNoCodeForAnUnrelatedFailure(t *testing.T) {
	assert.Empty(t, agentusecase.PromptErrorCode(errors.New("disk gone")))
	assert.Empty(t, agentusecase.PromptErrorCode(nil))
}

func TestCatalogErrorCode_NamesEveryCatalogueFailure(t *testing.T) {
	testCases := []struct {
		err  error
		want string
	}{
		{agentusecase.ErrSlashCatalogUnsupported, agentusecase.CatalogCodeUnsupported},
		{agentusecase.ErrSlashCatalogNoLiveTUI, agentusecase.CatalogCodeLiveRequired},
		{agentusecase.ErrSlashCatalogTimeout, agentusecase.CatalogCodeTimeout},
		{agentusecase.ErrSlashCatalogUnavailable, agentusecase.CatalogCodeUnavailable},
		{agentusecase.ErrSlashCatalogOutputLimit, agentusecase.CatalogCodeOutputLimit},
		{agentusecase.ErrSlashCatalogCommand, agentusecase.CatalogCodeCommand},
		{agentusecase.ErrSlashCatalogMalformed, agentusecase.CatalogCodeMalformed},
		{agentusecase.ErrSlashCatalogSuperseded, agentusecase.CatalogCodeSuperseded},
	}
	seen := map[string]struct{}{}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, agentusecase.CatalogErrorCode(tc.err))
			assert.Equal(t, tc.want,
				agentusecase.CatalogErrorCode(fmt.Errorf("agent: slash catalog: %w", tc.err)))
		})
		_, dup := seen[tc.want]
		assert.False(t, dup, "codes must be distinct: %s", tc.want)
		seen[tc.want] = struct{}{}
	}
}

func TestCatalogErrorCode_HasNoCodeForAnUnrelatedFailure(t *testing.T) {
	assert.Empty(t, agentusecase.CatalogErrorCode(errors.New("disk gone")))
	assert.Empty(t, agentusecase.CatalogErrorCode(nil))
}
