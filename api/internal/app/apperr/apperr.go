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
