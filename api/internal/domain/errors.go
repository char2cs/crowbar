package domain

import "errors"

// ErrNotFound is returned by repository methods when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrBadRequest is returned when the caller supplies invalid input.
var ErrBadRequest = errors.New("bad request")
