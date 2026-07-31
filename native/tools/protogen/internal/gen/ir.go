// Package gen implements protogen: it loads the Crowbar Go daemon with
// go/packages, discovers every gin route and the DTOs its handler binds and
// writes, closes transitively over those DTOs' field types, and emits Rust serde
// structs, TypeScript declarations and a JSON manifest from one source of truth.
//
// The intermediate representation lives here. Everything downstream of type
// resolution (the Rust emitter, the TypeScript emitter, the manifest) reads only
// this IR, so the two emitters can never disagree about optionality, field names
// or enum membership.
package gen

import (
	"sort"
	"strings"
)

// Kind discriminates the shapes a resolved Go type can take in the IR.
type Kind string

const (
	// KindStruct is a Go struct: an object on the wire.
	KindStruct Kind = "struct"
	// KindEnum is a named Go string type with a package-level constant set.
	KindEnum Kind = "enum"
	// KindAlias is a named Go type whose underlying type is a primitive, a
	// slice, or a map with no constant set to make it an enum.
	KindAlias Kind = "alias"
)

// TypeRef is a resolved reference to a type, already lowered to the shape the
// emitters need. Prim is set for primitives; Module+Name for references to
// another emitted declaration; Elem/Key for containers.
type TypeRef struct {
	// Prim is one of the primitive tokens ("string", "bool", "i64", "f64",
	// "json", ...) when this reference is not a named declaration or container.
	Prim string `json:"prim,omitempty"`
	// Module is the emitted module a named declaration lives in.
	Module string `json:"module,omitempty"`
	// Name is the emitted declaration name.
	Name string `json:"name,omitempty"`
	// Container is "", "slice", "map", or "option".
	Container string `json:"container,omitempty"`
	// Key is the map key reference (map only).
	Key *TypeRef `json:"key,omitempty"`
	// Elem is the element reference (slice/map/option only).
	Elem *TypeRef `json:"elem,omitempty"`
}

// Field is one JSON-visible field of a struct, after encoding/json's rules for
// embedding, tags and omitempty have been applied.
type Field struct {
	// JSONName is the exact key on the wire.
	JSONName string `json:"jsonName"`
	// GoName is the Go field name it came from, for traceability.
	GoName string `json:"goName"`
	// Type is the field's resolved type.
	Type TypeRef `json:"type"`
	// Optional is true when the field may be absent or null on the wire:
	// encoding/json omitempty, or a pointer field.
	Optional bool `json:"optional"`
	// Doc is the Go doc comment, already stripped of its comment markers.
	Doc string `json:"doc,omitempty"`
}

// EnumVariant is one member of a detected Go string-constant set.
type EnumVariant struct {
	// GoName is the Go constant identifier.
	GoName string `json:"goName"`
	// Value is the constant's string value — the wire representation.
	Value string `json:"value"`
	// Doc is the constant's doc comment.
	Doc string `json:"doc,omitempty"`
}

// Decl is one emitted declaration.
type Decl struct {
	// Module is the emitted module name (one per Go package).
	Module string `json:"module"`
	// Name is the emitted declaration name (the Go type name, verbatim).
	Name string `json:"name"`
	// GoPath is the fully qualified Go type, for traceability.
	GoPath string `json:"goPath"`
	// Kind is struct, enum or alias.
	Kind Kind `json:"kind"`
	// Doc is the Go doc comment.
	Doc string `json:"doc,omitempty"`
	// Fields is set for KindStruct.
	Fields []Field `json:"fields,omitempty"`
	// Variants is set for KindEnum.
	Variants []EnumVariant `json:"variants,omitempty"`
	// Underlying is set for KindAlias.
	Underlying *TypeRef `json:"underlying,omitempty"`
	// Generic marks a declaration emitted with one type parameter T (the
	// response envelope).
	Generic bool `json:"generic,omitempty"`
	// Synthetic marks a declaration protogen named itself because the Go side
	// used an anonymous struct.
	Synthetic bool `json:"synthetic,omitempty"`
	// Dropped lists the fields protogen could not lower and therefore left
	// out of this declaration. A struct with a non-empty Dropped is NOT the
	// full wire shape, and every endpoint that reaches it is reported
	// unresolved.
	Dropped []Unresolved `json:"dropped,omitempty"`
}

// Unresolved records one thing protogen could not turn into a DTO, and why.
// It is the honest half of the manifest: nothing is silently dropped.
type Unresolved struct {
	// What identifies the thing (an endpoint, a handler, a type, a field).
	What string `json:"what"`
	// Where is the Go source position, when known.
	Where string `json:"where,omitempty"`
	// Reason is the structural cause, phrased so it names what to fix.
	Reason string `json:"reason"`
	// Category groups reasons for the summary counts.
	Category string `json:"category"`
	// Severity is SeverityError when protogen emitted nothing for the item,
	// or SeverityWarning when it emitted a best-effort shape a human must
	// confirm.
	Severity string `json:"severity"`
}

const (
	// SeverityError marks an item protogen refused to generate at all.
	SeverityError = "error"
	// SeverityWarning marks an item protogen generated best-effort, whose
	// wire shape it cannot prove.
	SeverityWarning = "warning"
)

// ResponseKind classifies what a handler writes on its success path.
type ResponseKind string

const (
	// RespJSON is an envelope with a typed data payload.
	RespJSON ResponseKind = "json"
	// RespMutation is the fixed mutation envelope, data: {"id": string}.
	RespMutation ResponseKind = "mutation"
	// RespEmpty is a status-only response with no body (202/204/200-no-body).
	RespEmpty ResponseKind = "empty"
	// RespBinary is a raw non-JSON body written with c.Data.
	RespBinary ResponseKind = "binary"
	// RespWebSocket is a WebSocket upgrade route with no REST DTO.
	RespWebSocket ResponseKind = "websocket"
	// RespUnknown means protogen could not classify the response.
	RespUnknown ResponseKind = "unknown"
)

// Endpoint is one discovered (method, path, handler) triple with its DTOs.
type Endpoint struct {
	// Method is the HTTP method.
	Method string `json:"method"`
	// Path is the full registered path including every group prefix.
	Path string `json:"path"`
	// GoPackage is the package the handler is declared in.
	GoPackage string `json:"goPackage,omitempty"`
	// Handler is the handler function's Go name.
	Handler string `json:"handler,omitempty"`
	// HandlerPos is the handler's source position.
	HandlerPos string `json:"handlerPos,omitempty"`
	// Request is the request body type, nil when the handler binds no body.
	Request *TypeRef `json:"request,omitempty"`
	// Responses are the distinct success payload types the handler writes.
	Responses []TypeRef `json:"responses,omitempty"`
	// ResponseKind classifies the success response.
	ResponseKind ResponseKind `json:"responseKind"`
	// Doc is the handler's doc comment.
	Doc string `json:"doc,omitempty"`
	// Unresolved lists everything about THIS endpoint that did not resolve.
	Unresolved []Unresolved `json:"unresolved,omitempty"`
}

// Errors returns only the hard failures among an endpoint's diagnostics.
func (e Endpoint) Errors() []Unresolved {
	out := []Unresolved{}
	for _, u := range e.Unresolved {
		if u.Severity != SeverityWarning {
			out = append(out, u)
		}
	}
	return out
}

// FullyResolved reports whether both halves of the endpoint's contract are
// known: nothing about it failed to resolve, and its success response is
// classified (a typed payload, the mutation shape, an intentionally empty body,
// a raw binary body, or a WebSocket upgrade).
func (e Endpoint) FullyResolved() bool {
	return len(e.Errors()) == 0 && e.ResponseKind != RespUnknown
}

// Result is everything one protogen run produced.
type Result struct {
	// Endpoints is every discovered route, sorted by path then method.
	Endpoints []Endpoint
	// Decls is every emitted declaration, sorted by module then name.
	Decls []Decl
	// Unresolved is every failure across the whole run.
	Unresolved []Unresolved
}

// sortResult puts the result into its canonical order. Every ordering in the
// emitted output derives from here, so two runs over the same tree produce
// byte-identical files.
func (r *Result) sortResult() {
	sort.SliceStable(r.Endpoints, func(i, j int) bool {
		if r.Endpoints[i].Path != r.Endpoints[j].Path {
			return r.Endpoints[i].Path < r.Endpoints[j].Path
		}
		return r.Endpoints[i].Method < r.Endpoints[j].Method
	})
	sort.SliceStable(r.Decls, func(i, j int) bool {
		if r.Decls[i].Module != r.Decls[j].Module {
			return r.Decls[i].Module < r.Decls[j].Module
		}
		return r.Decls[i].Name < r.Decls[j].Name
	})
	sort.SliceStable(r.Unresolved, func(i, j int) bool {
		if r.Unresolved[i].Category != r.Unresolved[j].Category {
			return r.Unresolved[i].Category < r.Unresolved[j].Category
		}
		if r.Unresolved[i].What != r.Unresolved[j].What {
			return r.Unresolved[i].What < r.Unresolved[j].What
		}
		return r.Unresolved[i].Reason < r.Unresolved[j].Reason
	})
}

// Modules returns the sorted set of module names that carry declarations.
func (r *Result) Modules() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range r.Decls {
		if !seen[d.Module] {
			seen[d.Module] = true
			out = append(out, d.Module)
		}
	}
	sort.Strings(out)
	return out
}

// DeclsFor returns the declarations of one module, already sorted.
func (r *Result) DeclsFor(
	module string,
) []Decl {
	out := []Decl{}
	for _, d := range r.Decls {
		if d.Module == module {
			out = append(out, d)
		}
	}
	return out
}

// docLines splits a doc comment into the lines an emitter should write, with
// trailing blank lines removed so output is stable.
func docLines(
	doc string,
) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(doc, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
