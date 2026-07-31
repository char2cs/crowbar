package gen

import "testing"

// TestDiscoverRoutes_FixtureSurface asserts the walker reproduces gin's own
// path arithmetic: group prefixes accumulate across a Group chain and across a
// cross-package Register call, an empty relative path resolves to the group
// itself, and a dual-served route resolves to its REST half rather than to the
// dispatch wrapper.
func TestDiscoverRoutes_FixtureSurface(t *testing.T) {
	r := fixtureRun(t)

	tests := []struct {
		name    string
		method  string
		path    string
		handler string
		kind    ResponseKind
	}{
		{
			name:    "top-level route stays on the entry group",
			method:  "GET",
			path:    "/v0/health",
			handler: "Untyped",
			kind:    RespJSON,
		},
		{
			name:    "empty relative path resolves to the group itself",
			method:  "GET",
			path:    "/v0/projects/:projectId/items",
			handler: "ListItems",
			kind:    RespJSON,
		},
		{
			name:    "nested group prefixes accumulate across packages",
			method:  "GET",
			path:    "/v0/projects/:projectId/items/:id",
			handler: "GetItem",
			kind:    RespJSON,
		},
		{
			name:    "post binds a named body",
			method:  "POST",
			path:    "/v0/projects/:projectId/items",
			handler: "CreateItem",
			kind:    RespJSON,
		},
		{
			name:    "mutation response is the fixed id envelope",
			method:  "PATCH",
			path:    "/v0/projects/:projectId/items/:id",
			handler: "RenameItem",
			kind:    RespMutation,
		},
		{
			name:    "accepted response carries no body",
			method:  "DELETE",
			path:    "/v0/projects/:projectId/items/:id",
			handler: "DeleteItem",
			kind:    RespEmpty,
		},
		{
			name:    "raw body is classified binary",
			method:  "GET",
			path:    "/v0/projects/:projectId/patch",
			handler: "Patch",
			kind:    RespBinary,
		},
		{
			name:    "injected handler value is a websocket upgrade",
			method:  "GET",
			path:    "/v0/projects/:projectId/items/ws",
			handler: "",
			kind:    RespWebSocket,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := findEndpoint(t, r, tt.method, tt.path)
			if e.Handler != tt.handler {
				t.Errorf("handler = %q, want %q", e.Handler, tt.handler)
			}
			if e.ResponseKind != tt.kind {
				t.Errorf("responseKind = %q, want %q", e.ResponseKind, tt.kind)
			}
		})
	}
}

// TestDiscoverRoutes_DualServeResolvesRestHalf pins the dispatch unwrapping
// specifically: the terminal argument is a call, and taking it literally would
// leave every live-read route without a DTO.
func TestDiscoverRoutes_DualServeResolvesRestHalf(t *testing.T) {
	r := fixtureRun(t)
	e := findEndpoint(t, r, "GET", "/v0/projects/:projectId/items")
	if e.Handler != "ListItems" {
		t.Fatalf("dual-served route resolved to %q, want ListItems", e.Handler)
	}
	if len(e.Responses) != 1 || e.Responses[0].Container != "slice" {
		t.Fatalf("responses = %v, want one slice payload", e.Responses)
	}
	if e.Responses[0].Elem == nil || e.Responses[0].Elem.Name != "Item" {
		t.Fatalf("payload element = %v, want Item", e.Responses[0].Elem)
	}
}

// TestDiscoverRoutes_NoRequestBodyOnReads asserts a read endpoint reports no
// request type at all rather than an empty one.
func TestDiscoverRoutes_NoRequestBodyOnReads(t *testing.T) {
	r := fixtureRun(t)
	for _, path := range []string{
		"/v0/projects/:projectId/items",
		"/v0/projects/:projectId/items/:id",
		"/v0/projects/:projectId/tree",
	} {
		e := findEndpoint(t, r, "GET", path)
		if e.Request != nil {
			t.Errorf("GET %s: request = %v, want none", path, e.Request)
		}
	}
}

// TestDiscoverRoutes_RequestTypes covers both request shapes: a named Go
// struct, an anonymous one named after its handler, and a bind that happens one
// helper call away from the handler body.
func TestDiscoverRoutes_RequestTypes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		module string
		typ    string
	}{
		{
			name:   "named request struct",
			method: "POST",
			path:   "/v0/projects/:projectId/items",
			module: "fixture_types",
			typ:    "CreateItemBody",
		},
		{
			name:   "anonymous request struct is named after its handler",
			method: "PATCH",
			path:   "/v0/projects/:projectId/items/:id",
			module: "fixture_handlers",
			typ:    "RenameItemRequest",
		},
		{
			name:   "bind through a helper is still found",
			method: "POST",
			path:   "/v0/projects/:projectId/stage",
			module: "fixture_handlers",
			typ:    "StageRequest",
		},
	}
	r := fixtureRun(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := findEndpoint(t, r, tt.method, tt.path)
			if e.Request == nil {
				t.Fatalf("no request type resolved")
			}
			if e.Request.Module != tt.module || e.Request.Name != tt.typ {
				t.Fatalf("request = %s.%s, want %s.%s",
					e.Request.Module, e.Request.Name, tt.module, tt.typ)
			}
		})
	}
}

// TestDiscoverRoutes_UntypedPayloadIsReported asserts a gin.H response is
// reported as an unresolved endpoint rather than quietly emitted as an opaque
// map: an endpoint with no static shape is exactly what the manifest exists to
// surface.
func TestDiscoverRoutes_UntypedPayloadIsReported(t *testing.T) {
	r := fixtureRun(t)
	e := findEndpoint(t, r, "GET", "/v0/projects/:projectId/untyped")
	if e.FullyResolved() {
		t.Fatalf("gin.H endpoint reported as fully resolved")
	}
	if !hasCategory(e.Unresolved, "untyped-payload") {
		t.Fatalf("unresolved = %v, want an untyped-payload entry", e.Unresolved)
	}
}

// TestJoinPath covers gin's path arithmetic, including the empty relative path
// a group-root route registers with.
func TestJoinPath(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rel    string
		want   string
	}{
		{name: "root and empty", prefix: "", rel: "", want: "/"},
		{name: "prefix and empty", prefix: "/v0/items", rel: "", want: "/v0/items"},
		{name: "prefix and leading slash", prefix: "/v0", rel: "/items", want: "/v0/items"},
		{name: "prefix and bare segment", prefix: "/v0", rel: "items", want: "/v0/items"},
		{name: "trailing slash is not doubled", prefix: "/v0/", rel: "/items", want: "/v0/items"},
		{name: "param segments survive", prefix: "/v0/:id", rel: "/x", want: "/v0/:id/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinPath(tt.prefix, tt.rel); got != tt.want {
				t.Errorf("joinPath(%q, %q) = %q, want %q", tt.prefix, tt.rel, got, tt.want)
			}
		})
	}
}

// TestStreamedJSONPayloadIsNotAnEmptyBody pins the classification of a handler
// that writes its envelope with `json.NewEncoder(w).Encode(v)` instead of
// through gin or a libs helper.
//
// Found in the daemon rather than designed for: `GET /v0/.../review/outline`
// streams its response because it is the one v0 payload large enough that
// gin's marshal-then-write would hold the whole thing in memory. Because it
// touches neither `ctx.JSON` nor `libs.Write*`, the classifier fell through to
// "body-less success" — and reported no diagnostic, so three real wire types
// (`outlineResponse`, `git.FileOutline`, `git.HunkShape`) were absent from the
// generated crate with nothing anywhere saying they were missing. §9.2's
// promise is that nothing is dropped silently; `empty` is the one wrong answer
// that looks exactly like a right one in every count the summary prints.
//
// The fixture's Outline handler reproduces the shape that hid it: the encoder
// is two same-package helpers away from the handler, and the payload arrives
// there as a parameter.
func TestStreamedJSONPayloadIsNotAnEmptyBody(t *testing.T) {
	r := fixtureRun(t)
	e := findEndpoint(t, r, "GET", "/v0/projects/:projectId/outline")

	if e.ResponseKind != RespJSON {
		t.Fatalf("response kind %q, want %q — a streamed envelope is a body, not the absence of one",
			e.ResponseKind, RespJSON)
	}
	if len(e.Responses) != 1 {
		t.Fatalf("resolved %d response payloads, want 1: %+v", len(e.Responses), e.Responses)
	}
	if got := e.Responses[0].Name; got != "OutlineResponse" {
		t.Errorf("payload type %q, want OutlineResponse", got)
	}
	if len(e.Unresolved) != 0 {
		t.Errorf("endpoint reported unresolved: %+v", e.Unresolved)
	}

	// And the payload's own field types have to be pulled into the closure,
	// which is the half that actually went missing.
	d := findDecl(t, r, "OutlineResponse")
	f := findField(t, d, "files")
	if f.Type.Container != "slice" || f.Type.Elem == nil || f.Type.Elem.Name != "Nested" {
		t.Errorf("files field resolved to %+v, want a slice of Nested", f.Type)
	}
}
