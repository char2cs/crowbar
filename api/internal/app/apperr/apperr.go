// Package apperr defines app-layer sentinel errors that usecases return so the
// API layer can map them to HTTP status codes without depending on the
// underlying persistence engine's error vocabulary.
package apperr

import "errors"

// ErrNotFound signals that a requested entity (workspace, thread, repo row, …)
// genuinely does not exist. Usecases wrap engine-level not-found errors with
// this sentinel so handlers can distinguish a 404 from a 500 via errors.Is.
var ErrNotFound = errors.New("apperr: not found")
