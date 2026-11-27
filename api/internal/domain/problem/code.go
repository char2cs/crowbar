package problem

import (
	"net/http"
	"slices"
)

type Code string

const (
	InvalidArgument  Code = "invalid_argument"
	Unauthorized     Code = "unauthorized"
	Forbidden        Code = "forbidden"
	NotFound         Code = "not_found"
	Conflict         Code = "conflict"
	Locked           Code = "locked"
	TooLarge         Code = "too_large"
	Unprocessable    Code = "unprocessable"
	FailedDependency Code = "failed_dependency"
	Internal         Code = "internal"
	BadGateway       Code = "bad_gateway"
	Unavailable      Code = "unavailable"
	Timeout          Code = "timeout"
)

const (
	ChatBusy                Code = "chat_busy"
	RequestOutcomeUncertain Code = "request_outcome_uncertain"
	RequestIDConflict       Code = "request_id_conflict"
	PromptSubmitUnsupported Code = "prompt_submit_unsupported"
	LiveTUIRequired         Code = "live_tui_required"
	SessionLost             Code = "session_lost"
	PromptWithProvider      Code = "prompt_with_provider"

	CatalogUnsupported        Code = "catalog_unsupported"
	CatalogTimeout            Code = "catalog_timeout"
	CatalogCommandUnavailable Code = "catalog_command_unavailable"
	CatalogOutputLimit        Code = "catalog_output_limit"
	CatalogCommandFailed      Code = "catalog_command_failed"
	CatalogMalformedOutput    Code = "catalog_malformed_output"
	CatalogSuperseded         Code = "catalog_superseded"

	HostDocumentRequiresLocalTransport Code = "host_document_requires_local_transport"
	DaemonStateDocument                Code = "daemon_state_document"

	WorkAtRisk          Code = "work_at_risk"
	IDConflict          Code = "id_conflict"
	OperationIDConflict Code = "operation_id_conflict"
	OperationRunning    Code = "operation_running"

	TreeCycle          Code = "tree_cycle"
	TreeNotAContainer  Code = "tree_not_a_container"
	TreeCrossContext   Code = "tree_cross_context"
	TreeCrossWorkspace Code = "tree_cross_workspace"
	TreeForkChain      Code = "tree_fork_chain"
	TreeSubtreeWorking Code = "tree_subtree_working"

	WatchLimit Code = "watch_limit"

	DaemonMigrating         Code = "daemon_migrating"
	DaemonHalted            Code = "daemon_halted"
	DaemonReportUnencodable Code = "daemon_report_unencodable"

	Interrupted Code = "interrupted"
	Partial     Code = "partial"
)

func (c Code) Status() int {
	switch c {
	case InvalidArgument:
		return http.StatusBadRequest
	case Unauthorized:
		return http.StatusUnauthorized
	case Forbidden, HostDocumentRequiresLocalTransport, DaemonStateDocument:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict, Locked, ChatBusy, SessionLost, PromptWithProvider, RequestOutcomeUncertain, RequestIDConflict,
		CatalogSuperseded, WorkAtRisk, IDConflict, OperationIDConflict, OperationRunning,
		TreeCycle, TreeNotAContainer, TreeCrossContext, TreeCrossWorkspace, TreeForkChain, TreeSubtreeWorking:
		return http.StatusConflict
	case TooLarge:
		return http.StatusRequestEntityTooLarge
	case Unprocessable, PromptSubmitUnsupported, LiveTUIRequired, CatalogUnsupported:
		return http.StatusUnprocessableEntity
	case FailedDependency, CatalogCommandUnavailable:
		return http.StatusFailedDependency
	case BadGateway, CatalogOutputLimit, CatalogCommandFailed, CatalogMalformedOutput:
		return http.StatusBadGateway
	case WatchLimit:
		return http.StatusInsufficientStorage
	case Unavailable, DaemonMigrating, DaemonHalted:
		return http.StatusServiceUnavailable
	case Timeout, CatalogTimeout:
		return http.StatusGatewayTimeout
	case Internal, DaemonReportUnencodable, Interrupted, Partial:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

var categories = []Code{
	InvalidArgument, Unauthorized, Forbidden, NotFound, Conflict, TooLarge, Unprocessable,
	FailedDependency, BadGateway, Unavailable, Timeout,
}

func (c Code) isCategory() bool {
	return c == Locked || c == Internal || slices.Contains(categories, c)
}

func CategoryFor(status int) Code {
	for _, c := range categories {
		if c.Status() == status {
			return c
		}
	}
	return Internal
}

var rank = map[int]int{
	http.StatusNotFound:              0,
	http.StatusBadRequest:            1,
	http.StatusRequestEntityTooLarge: 2,
	http.StatusForbidden:             3,
	http.StatusFailedDependency:      4,
	http.StatusUnprocessableEntity:   5,
	http.StatusGatewayTimeout:        6,
	http.StatusBadGateway:            7,
	http.StatusServiceUnavailable:    8,
	http.StatusConflict:              9,
}

func (c Code) rank() int {
	if r, ok := rank[c.Status()]; ok {
		return r
	}
	return len(rank)
}
