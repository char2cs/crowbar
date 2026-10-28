package problem

import "fmt"

// Problem is a failure as the client reads it.
type Problem struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// Error is an error that names its code. Data rides to the client beside the
// message; Err is what it wraps, so a coded sentinel can still match its category.
type Error struct {
	Code    Code
	Message string
	Data    any
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.Err == nil:
		return e.Message
	case e.Message == "":
		return e.Err.Error()
	default:
		return e.Message + ": " + e.Err.Error()
	}
}

func (e *Error) Unwrap() error { return e.Err }

// New is a coded error with no cause: the form of a sentinel.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Errorf is a coded error built like fmt.Errorf; a %w operand stays in its chain.
func Errorf(code Code, format string, args ...any) error {
	wrapped := fmt.Errorf(format, args...)
	return &Error{Code: code, Err: wrapped}
}

// The category sentinels.
var (
	ErrNotFound         = New(NotFound, "not found")
	ErrLocked           = New(Locked, "workspace locked")
	ErrInvalidArgument  = New(InvalidArgument, "invalid argument")
	ErrUnavailable      = New(Unavailable, "service unavailable")
	ErrConflict         = New(Conflict, "conflict")
	ErrUnprocessable    = New(Unprocessable, "unprocessable entity")
	ErrTimeout          = New(Timeout, "gateway timeout")
	ErrBadGateway       = New(BadGateway, "bad gateway")
	ErrFailedDependency = New(FailedDependency, "failed dependency")
)

// CodeOf is the code a response for err carries. Of every code in err's tree,
// the one whose category ranks first wins; on a tie, the outermost. None is Internal.
func CodeOf(err error) Code {
	best := Internal
	walk(err, func(c Code) {
		if c.rank() < best.rank() {
			best = c
		}
	})
	return best
}

// Of is err as the client reads it.
func Of(err error) Problem {
	if err == nil {
		return Problem{Code: Internal, Message: "internal error"}
	}
	return Problem{Code: CodeOf(err), Message: err.Error()}
}

// DataOf is the Data of the first coded error in err's tree that has any.
func DataOf(err error) any {
	var data any
	visit(err, func(node error) bool {
		if e, ok := node.(*Error); ok && e.Data != nil { //nolint:errorlint // one node of a tree visit walks
			data = e.Data
			return true
		}
		return false
	})
	return data
}

// walk calls found with each code in err's tree, outermost first.
func walk(err error, found func(Code)) {
	visit(err, func(node error) bool {
		if e, ok := node.(*Error); ok { //nolint:errorlint // one node of a tree visit walks
			found(e.Code)
		}
		if c, ok := foreignCode(node); ok {
			found(c)
		}
		return false
	})
}

// visit walks err's tree depth first, outermost first, until stop says so.
func visit(err error, stop func(error) bool) bool {
	if err == nil {
		return false
	}
	if stop(err) {
		return true
	}
	switch u := err.(type) { //nolint:errorlint // walking the tree by hand is the point
	case interface{ Unwrap() []error }:
		for _, child := range u.Unwrap() {
			if visit(child, stop) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		return visit(u.Unwrap(), stop)
	}
	return false
}

// is reports whether node itself, without unwrapping, is target.
func is(node, target error) bool {
	if node == target { //nolint:errorlint // one node of a tree the caller walks
		return true
	}
	m, ok := node.(interface{ Is(error) bool })
	return ok && m.Is(target)
}
