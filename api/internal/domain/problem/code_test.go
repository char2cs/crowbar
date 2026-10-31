package problem_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestCode_StatusTable(t *testing.T) {
	cases := map[problem.Code]int{
		problem.InvalidArgument:  http.StatusBadRequest,
		problem.Forbidden:        http.StatusForbidden,
		problem.NotFound:         http.StatusNotFound,
		problem.Conflict:         http.StatusConflict,
		problem.Locked:           http.StatusConflict,
		problem.TooLarge:         http.StatusRequestEntityTooLarge,
		problem.Unprocessable:    http.StatusUnprocessableEntity,
		problem.FailedDependency: http.StatusFailedDependency,
		problem.Internal:         http.StatusInternalServerError,
		problem.BadGateway:       http.StatusBadGateway,
		problem.Unavailable:      http.StatusServiceUnavailable,
		problem.Timeout:          http.StatusGatewayTimeout,

		problem.ChatBusy:                http.StatusConflict,
		problem.RequestOutcomeUncertain: http.StatusConflict,
		problem.RequestIDConflict:       http.StatusConflict,
		problem.PromptSubmitUnsupported: http.StatusUnprocessableEntity,
		problem.LiveTUIRequired:         http.StatusUnprocessableEntity,
		problem.SessionLost:             http.StatusConflict,
		problem.PromptWithProvider:      http.StatusConflict,

		problem.CatalogUnsupported:        http.StatusUnprocessableEntity,
		problem.CatalogTimeout:            http.StatusGatewayTimeout,
		problem.CatalogCommandUnavailable: http.StatusFailedDependency,
		problem.CatalogOutputLimit:        http.StatusBadGateway,
		problem.CatalogCommandFailed:      http.StatusBadGateway,
		problem.CatalogMalformedOutput:    http.StatusBadGateway,
		problem.CatalogSuperseded:         http.StatusConflict,

		problem.HostDocumentRequiresLocalTransport: http.StatusForbidden,
		problem.DaemonStateDocument:                http.StatusForbidden,

		problem.WorkAtRisk:          http.StatusConflict,
		problem.IDConflict:          http.StatusConflict,
		problem.OperationIDConflict: http.StatusConflict,
		problem.OperationRunning:    http.StatusConflict,

		problem.TreeCycle:          http.StatusConflict,
		problem.TreeNotAContainer:  http.StatusConflict,
		problem.TreeCrossContext:   http.StatusConflict,
		problem.TreeCrossWorkspace: http.StatusConflict,
		problem.TreeForkChain:      http.StatusConflict,
		problem.TreeSubtreeWorking: http.StatusConflict,

		problem.DaemonMigrating:         http.StatusServiceUnavailable,
		problem.DaemonHalted:            http.StatusServiceUnavailable,
		problem.DaemonReportUnencodable: http.StatusInternalServerError,

		problem.Interrupted: http.StatusInternalServerError,
		problem.Partial:     http.StatusInternalServerError,
	}
	for code, status := range cases {
		assert.Equal(t, status, code.Status(), code)
	}
}

func TestCode_UnknownCodeIsInternal(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, problem.Code("nope").Status())
}

func TestCategoryFor_NamesTheCategoryOfAStatus(t *testing.T) {
	assert.Equal(t, problem.NotFound, problem.CategoryFor(http.StatusNotFound))
	assert.Equal(t, problem.Conflict, problem.CategoryFor(http.StatusConflict))
	assert.Equal(t, problem.Internal, problem.CategoryFor(http.StatusTeapot))
}
