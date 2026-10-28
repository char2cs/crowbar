// Package problem is the closed vocabulary of failures the daemon reports: one
// code per failure a client can tell apart, and the HTTP status each maps to.
package problem

import "net/http"

// Code is a failure's machine-readable name. The set is closed: every Code is
// one of the constants below, spelled as the client branches on it.
type Code string

// The category codes. Every specific code below ranks as one of them.
const (
	InvalidArgument  Code = "invalid_argument"
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

// The specific codes: refusals whose recovery differs from their category's.
const (
	ChatBusy                Code = "chat_busy"
	RequestOutcomeUncertain Code = "request_outcome_uncertain"
	RequestIDConflict       Code = "request_id_conflict"
	PromptSubmitUnsupported Code = "prompt_submit_unsupported"
	LiveTUIRequired         Code = "live_tui_required"

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

	// Written by core/bootgate itself, which stays free of dependencies.
	DaemonMigrating         Code = "daemon_migrating"
	DaemonHalted            Code = "daemon_halted"
	DaemonReportUnencodable Code = "daemon_report_unencodable"

	// An operation's outcome only; never a response's.
	Interrupted Code = "interrupted"
	Partial     Code = "partial"
)

// Status is the HTTP status a response carrying c answers with.
func (c Code) Status() int {
	switch c {
	case InvalidArgument:
		return http.StatusBadRequest
	case Forbidden, HostDocumentRequiresLocalTransport, DaemonStateDocument:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict, Locked, ChatBusy, RequestOutcomeUncertain, RequestIDConflict,
		CatalogSuperseded, WorkAtRisk, IDConflict, OperationIDConflict, OperationRunning:
		return http.StatusConflict
	case TooLarge:
		return http.StatusRequestEntityTooLarge
	case Unprocessable, PromptSubmitUnsupported, LiveTUIRequired, CatalogUnsupported:
		return http.StatusUnprocessableEntity
	case FailedDependency, CatalogCommandUnavailable:
		return http.StatusFailedDependency
	case BadGateway, CatalogOutputLimit, CatalogCommandFailed, CatalogMalformedOutput:
		return http.StatusBadGateway
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

// CategoryFor is the category code of a bare status, for a response written
// with a status and no error.
func CategoryFor(status int) Code {
	for _, c := range []Code{
		InvalidArgument, Forbidden, NotFound, Conflict, TooLarge, Unprocessable,
		FailedDependency, BadGateway, Unavailable, Timeout,
	} {
		if c.Status() == status {
			return c
		}
	}
	return Internal
}

// rank orders codes as the old status union did, so a chain holding several
// keeps the status it answered before: NotFound first, Conflict last.
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
