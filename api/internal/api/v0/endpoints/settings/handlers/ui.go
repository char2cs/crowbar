package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// MaxValueBytes caps the request body a single scope may store, at 256 KiB.
//
// The four client stores this endpoint exists for are all small: the theme /
// font preference blob is ~200 bytes, the sidebar's collapsed-id lists run
// ~0.2-10 KB, the per-repo workspace hierarchy ~1-8 KB, and a pane tree with
// its buffer descriptors a few KB. 256 KiB is roughly 25x the largest of those,
// so no legitimate UI state comes near it.
//
// It is deliberately far BELOW the megabytes the web client's layout blob
// reaches today, because that blob is only that large for a reason this
// endpoint must not inherit: it embeds each open editor's full file text twice
// over plus its syntax-token array. UI state is geometry and identifiers. A
// client that starts shipping document CONTENT here should get a loud 413, not
// silent multi-megabyte writes into the daemon's global view.db on every
// keystroke-debounced save.
const MaxValueBytes = 256 << 10

// ScopeGlobal is the scope key for machine-wide UI state that is not tied to
// any project, repo or workspace.
const ScopeGlobal = "global"

const (
	scopeRepoPrefix      = "repo:"
	scopeWorkspacePrefix = "workspace:"
	maxScopeLen          = 128
	maxScopeIDLen        = 64
)

var (
	errScopeRequired = errors.New(
		`settings: "scope" query parameter is required ` +
			`(one of "global", "repo:<repoId>", "workspace:<workspaceId>")`,
	)
	errScopeMalformed = errors.New(
		`settings: invalid "scope": expected "global", "repo:<repoId>" or ` +
			`"workspace:<workspaceId>" with an id of 1-64 characters from [A-Za-z0-9._-]`,
	)
	errBodyNotObject = errors.New("settings: body must be a JSON object")
	errBodyTooLarge  = errors.New("settings: body exceeds the 256 KiB per-scope limit")
)

// emptyObject is what a GET returns for a scope nothing has been stored under.
// A first-run client must not have to special-case absence, so this is a 200
// rather than a 404: "no UI state yet" and "an empty UI state" are the same
// thing to the caller, and both mean "use your defaults".
var emptyObject = json.RawMessage("{}")

// GetUI handles GET /v0/settings/ui?scope=<scope>.
//
// It returns the JSON object last PUT under that scope, verbatim. A scope with
// nothing stored yields 200 with {} rather than 404. The value is opaque: the
// daemon returns exactly the bytes it was given (compacted), having never
// parsed past confirming it was an object.
func (h *Handlers) GetUI(
	ctx *gin.Context,
) {
	scope, err := parseScope(ctx.Query("scope"))
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	record, err := h.store.FindByKey(ctx.Request.Context(), scope)
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if record == nil || record.Value == "" {
		libs.WriteQueryOK(ctx, emptyObject)
		return
	}

	libs.WriteQueryOK(ctx, json.RawMessage(record.Value))
}

// PutUI handles PUT /v0/settings/ui?scope=<scope>.
//
// The body must be a JSON object; it REPLACES whatever was stored under that
// scope wholesale, so keys absent from the new body are dropped. The daemon
// does not merge, does not validate the object's shape, and does not know what
// any of its keys mean — that is the point, and it is what keeps client layout
// decisions out of the daemon. The stored object is echoed back so a client can
// confirm from server truth without a second read.
//
// A non-object body (array, scalar, null, malformed) is rejected 400; a body
// over MaxValueBytes is rejected 413.
func (h *Handlers) PutUI(
	ctx *gin.Context,
) {
	scope, err := parseScope(ctx.Query("scope"))
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	value, err := readObjectBody(ctx.Request)
	if err != nil {
		libs.WriteErr(ctx, statusForBodyErr(err), err.Error())
		return
	}

	mu := h.lockFor(scope)
	mu.Lock()
	defer mu.Unlock()

	record := domain.UISettings{Scope: scope, Value: string(value)}
	if saveErr := h.store.Save(ctx.Request.Context(), record); saveErr != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, saveErr.Error())
		return
	}

	libs.WriteQueryOK(ctx, json.RawMessage(value))
}

func statusForBodyErr(
	err error,
) int {
	if errors.Is(err, errBodyTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// parseScope validates the scope query parameter and returns it unchanged.
//
// Three forms are accepted and nothing else, so the table cannot quietly become
// a junk drawer of client-invented keys: "global" for machine-wide state,
// "repo:<repoId>" for per-repo state, and "workspace:<workspaceId>" for
// per-workspace state. The daemon understands the SCOPE — which is how it knows
// what a value belongs to — and nothing whatsoever about the value.
func parseScope(
	raw string,
) (string, error) {
	if raw == "" {
		return "", errScopeRequired
	}
	if len(raw) > maxScopeLen {
		return "", errScopeMalformed
	}
	if raw == ScopeGlobal {
		return raw, nil
	}
	if id, ok := strings.CutPrefix(raw, scopeRepoPrefix); ok {
		return scopeWithID(raw, id)
	}
	if id, ok := strings.CutPrefix(raw, scopeWorkspacePrefix); ok {
		return scopeWithID(raw, id)
	}
	return "", errScopeMalformed
}

func scopeWithID(
	raw string,
	id string,
) (string, error) {
	if id == "" || len(id) > maxScopeIDLen {
		return "", errScopeMalformed
	}
	if strings.ContainsFunc(id, isNotScopeIDRune) {
		return "", errScopeMalformed
	}
	return raw, nil
}

func isNotScopeIDRune(
	r rune,
) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= 'A' && r <= 'Z':
		return false
	case r >= '0' && r <= '9':
		return false
	case r == '-' || r == '_' || r == '.':
		return false
	}
	return true
}

// readObjectBody reads the request body under a hard byte cap and returns it
// compacted, having confirmed it is a JSON object.
//
// The cap is enforced on the wire, not after buffering: the body is read
// through a limit of MaxValueBytes+1 so an oversize (or Content-Length-lying,
// or chunked) request is rejected on the one extra byte instead of being
// materialised in full.
func readObjectBody(
	req *http.Request,
) ([]byte, error) {
	if req.Body == nil {
		return nil, errBodyNotObject
	}
	if req.ContentLength > MaxValueBytes {
		return nil, errBodyTooLarge
	}

	raw, err := io.ReadAll(io.LimitReader(req.Body, MaxValueBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxValueBytes {
		return nil, errBodyTooLarge
	}

	return compactObject(raw)
}

// compactObject rejects anything that is not a JSON object and returns the
// object with insignificant whitespace stripped.
//
// Decoding into a map is what makes the check total: an array, a number, a
// string and a malformed document all fail to unmarshal, and the one value that
// unmarshals without error — null — leaves the map nil and is caught by the nil
// guard. Compacting stores the smallest faithful representation while keeping
// member order and every nested value byte-identical, so a GET round-trips
// exactly what was PUT.
func compactObject(
	raw []byte,
) ([]byte, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, errBodyNotObject
	}
	if probe == nil {
		return nil, errBodyNotObject
	}

	var compacted bytes.Buffer
	if err := json.Compact(&compacted, raw); err != nil {
		return nil, errBodyNotObject
	}
	return compacted.Bytes(), nil
}
