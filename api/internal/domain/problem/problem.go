package problem

import (
	"errors"
	"slices"
	"strings"
)

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

var sentinels = []*Error{
	ErrNotFound, ErrLocked, ErrInvalidArgument, ErrUnavailable, ErrConflict,
	ErrUnprocessable, ErrTimeout, ErrBadGateway, ErrFailedDependency,
}

// CodeOf is the code a response for err carries. Of every code in err's tree,
// the one whose category ranks first wins; on a tie a specific code beats a
// category, then the outermost wins. None is Internal.
func CodeOf(err error) Code {
	best := Internal
	walk(err, func(c Code) {
		if c.rank() < best.rank() || c.rank() == best.rank() && best.isCategory() && !c.isCategory() {
			best = c
		}
	})
	return best
}

// Detailer is an error that carries what the tool it ran said, such as git's
// stderr: the part of a failure a user can act on.
type Detailer interface {
	Detail() string
}

const internalMessage = "internal error"

// Of is err as the client reads it: its code, and a message that starts at its
// outermost coded or tool error, never the wrap chain around it.
func Of(err error) Problem {
	if err == nil {
		return Problem{Code: Internal, Message: internalMessage}
	}
	return Problem{Code: CodeOf(err), Message: messageOf(err)}
}

// messageOf starts at the outermost coded or tool error, dropping the wraps
// above it unless one says more after it; a bare sentinel is told by the wrap
// that declared it. With none of those, every word is kept: a wrap may format what it was about.
func messageOf(err error) string {
	var outer error
	node := err
	for {
		if outer != nil && isSentinel(node) {
			return outer.Error()
		}
		switch n := node.(type) { //nolint:errorlint // the walk inspects each node itself
		case *Error:
			if n.Message != "" {
				return withCause(n.Message, n.Err)
			}
			if n.Err == nil {
				return internalMessage
			}
			outer, node = node, n.Err
			continue
		case Detailer:
			if d := strings.TrimSpace(n.Detail()); d != "" {
				return d
			}
		case interface{ Unwrap() []error }:
			if said, ok := joined(node, n.Unwrap()); ok {
				return said
			}
		}
		inner := errors.Unwrap(node)
		if inner == nil || !named(inner) {
			return orInternal(withDetails(err))
		}
		if !strings.HasSuffix(node.Error(), inner.Error()) {
			return orInternal(withDetails(node))
		}
		outer, node = node, inner
	}
}

// named reports whether err's tree holds a node that names the failure itself:
// a coded error, a tool error or a category sentinel.
func named(err error) bool {
	return visit(err, func(node error) bool {
		_, coded := node.(*Error) //nolint:errorlint // one node of a tree visit walks
		_, tool := node.(Detailer)
		return coded || tool || isSentinel(node)
	})
}

// joined is what each part of fmt.Errorf("%w: %w", a, b) says, when that is
// all node says; a join with words of its own is kept whole.
func joined(node error, parts []error) (string, bool) {
	texts := make([]string, len(parts))
	said := make([]string, len(parts))
	for i, part := range parts {
		texts[i], said[i] = part.Error(), messageOf(part)
	}
	if node.Error() != strings.Join(texts, ": ") {
		return "", false
	}
	return strings.Join(said, ": "), true
}

// isSentinel reports whether err is itself a category sentinel, Crowbar's or
// a foreign one: a name for the failure, not a description of it.
func isSentinel(err error) bool {
	if e, ok := err.(*Error); ok && slices.Contains(sentinels, e) { //nolint:errorlint // the node itself, not its tree
		return true
	}
	for _, f := range foreign {
		if err == f.err { //nolint:errorlint // the node itself, not its tree
			return true
		}
	}
	return false
}

// withCause is message followed by what its cause says, when that says anything.
func withCause(message string, cause error) string {
	if cause == nil || isSentinel(cause) {
		return message
	}
	if said := messageOf(cause); said != internalMessage {
		return message + ": " + said
	}
	return message
}

// withDetails is err's text with the first tool error in it cut to its output.
func withDetails(err error) string {
	text := err.Error()
	visit(err, func(node error) bool {
		d, ok := node.(Detailer)
		if !ok {
			return false
		}
		detail := strings.TrimSpace(d.Detail())
		if detail != "" {
			text = strings.Replace(text, node.Error(), detail, 1)
		}
		return detail != ""
	})
	return text
}

func orInternal(message string) string {
	if strings.TrimSpace(message) == "" {
		return internalMessage
	}
	return message
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
