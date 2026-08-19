// Package apperr defines app-layer sentinel errors that usecases return so the
// API layer can map them to HTTP status codes without depending on the
// underlying persistence engine's error vocabulary.
package apperr

import "errors"

// ErrNotFound signals that a requested entity (workspace, thread, repo row, …)
// genuinely does not exist. Usecases wrap engine-level not-found errors with
// this sentinel so handlers can distinguish a 404 from a 500 via errors.Is.
var ErrNotFound = errors.New("apperr: not found")

// ErrLocked signals that a write was attempted against a locked workspace.
// A workspace is locked when it tracks a provider-protected branch (04 §5,
// 05 §3/§4): the only mutation permitted on it is chat creation. The file and
// git write usecases wrap this sentinel so handlers map it to HTTP 409 via
// errors.Is, mirroring the engine-level enginesearch.ErrLocked guard already
// enforced on global replace.
var ErrLocked = errors.New("apperr: workspace locked")

// ErrInvalidArgument signals that a usecase received a syntactically invalid or
// unsafe argument from the client (e.g. a git operand that begins with "-" and
// could be interpreted as a command-line option, or a reset mode outside the
// allowlist). The git write usecase wraps this sentinel so handlers map it to
// HTTP 400 via errors.Is, rejecting the request before it reaches the engine.
var ErrInvalidArgument = errors.New("apperr: invalid argument")

// ErrUnavailable signals that a mutation could not be accepted because the
// backing asynx shard queue is full (asynxmodels.ErrQueueFull). Once the
// per-aggregate writeMu is gone (asynx-alignment refactor), one shared asynx
// instance absorbs concurrent Sends under load, so a shard queue can fill; the
// workspace repo wraps this sentinel so handlers map it to HTTP 503 via
// errors.Is, telling the client to retry.
var ErrUnavailable = errors.New("apperr: service unavailable")

// ErrConflict signals that a syntactically valid request conflicts with a
// concurrent or previously accepted operation. It maps to HTTP 409.
var ErrConflict = errors.New("apperr: conflict")

// ErrUnprocessable signals that a syntactically valid request cannot be
// performed for the entity's current capabilities or state. It maps to HTTP
// 422 without pretending the request body was malformed.
var ErrUnprocessable = errors.New("apperr: unprocessable entity")

// ErrTimeout signals that a bounded downstream operation exceeded its deadline.
// It maps to HTTP 504.
var ErrTimeout = errors.New("apperr: gateway timeout")

// ErrBadGateway signals that a deterministic external command returned a
// malformed or failed response. It maps to HTTP 502.
var ErrBadGateway = errors.New("apperr: bad gateway")

// ErrFailedDependency signals that something OUTSIDE the server, which the
// request depends on, failed — the user's vendor CLI above all. The request was
// well-formed and the daemon is healthy, so it is neither a 4xx the client can
// rephrase nor a 500 the server can be blamed for: it maps to HTTP 424 Failed
// Dependency, alongside engineterminal.ErrCommandNotFound (the CLI that is not
// installed) which was the first member of this class.
//
// It is the sentinel for a dependency that is present but did not work — a CLI
// that starts and dies on the spot — as opposed to one that is missing outright.
var ErrFailedDependency = errors.New("apperr: failed dependency")
