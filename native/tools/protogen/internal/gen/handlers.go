package gen

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"
)

// ginContext is the request context type; a bind or write call is only
// interesting when its receiver is one of these.
const ginContext = "github.com/gin-gonic/gin.Context"

// bindMethods are the gin Context methods that decode a JSON request body.
var bindMethods = map[string]bool{
	"ShouldBindJSON":     true,
	"BindJSON":           true,
	"ShouldBindWith":     true,
	"MustBindWith":       true,
	"ShouldBind":         true,
	"Bind":               true,
	"ShouldBindBodyWith": true,
}

// helperDepth bounds how far protogen follows same-package helper calls out of
// a handler. Two levels covers the daemon's bindPaths/writeX helpers without
// wandering into usecase internals.
const helperDepth = 2

// handlerFacts is what one handler body revealed about its wire contract.
type handlerFacts struct {
	// request is the bound request body type, nil when the handler binds none.
	request types.Type
	// responses are the distinct success payload types, in discovery order.
	responses []types.Type
	// responseOrigins parallels responses.
	responseOrigins []string
	// kind classifies the success response.
	kind ResponseKind
	// upgraded records that the handler hijacks the connection into a
	// WebSocket, which is a success path with no JSON body at all.
	upgraded bool
	// problems are the structural reasons something did not resolve.
	problems []Unresolved
}

// analyseHandler walks a handler body and reports its request/response facts.
func (a *analyzer) analyseHandler(
	site *funcSite,
	label string,
) handlerFacts {
	facts := handlerFacts{kind: RespUnknown}
	seen := map[types.Object]bool{}
	a.walkBody(site, label, 0, seen, &facts)
	if facts.kind == RespUnknown && len(facts.responses) > 0 {
		facts.kind = RespJSON
	}
	if facts.upgraded && facts.kind == RespUnknown {
		facts.kind = RespWebSocket
	}
	return facts
}

// analyzer resolves route handlers against the loaded program.
type analyzer struct {
	prog *Program
	res  *resolver
	root string
	// responsePkg is the import path of the package whose Write* helpers the
	// daemon answers every request through.
	responsePkg string
}

// walkBody inspects one function body for bind and response calls, following
// same-package helper calls up to helperDepth.
func (a *analyzer) walkBody(
	site *funcSite,
	label string,
	depth int,
	seen map[types.Object]bool,
	facts *handlerFacts,
) {
	if site.Decl.Body == nil {
		return
	}
	ast.Inspect(site.Decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if matchUpgrade(site, call) {
			facts.upgraded = true
			return true
		}
		if a.matchBind(site, call, label, facts) {
			return true
		}
		if a.matchResponse(site, call, label, facts) {
			return true
		}
		if depth < helperDepth {
			a.followHelper(site, call, label, depth, seen, facts)
		}
		return true
	})
}

// websocketPkgPath is the WebSocket library the daemon upgrades with. A
// handler that calls its Upgrader answers on a hijacked connection, so it has
// no JSON response body by construction rather than by omission.
const websocketPkgPath = "github.com/gorilla/websocket"

// matchUpgrade reports whether a call hijacks the connection into a WebSocket.
func matchUpgrade(
	site *funcSite,
	call *ast.CallExpr,
) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Upgrade" {
		return false
	}
	fn, ok := site.Pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == websocketPkgPath
}

// matchBind records a request body bound by a gin Context method. It reports
// whether the call was a bind call.
func (a *analyzer) matchBind(
	site *funcSite,
	call *ast.CallExpr,
	label string,
	facts *handlerFacts,
) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !bindMethods[sel.Sel.Name] {
		return false
	}
	if !isGinContextExpr(site, sel.X) {
		return false
	}
	if len(call.Args) == 0 {
		return true
	}
	arg := call.Args[0]
	t := site.Pkg.TypesInfo.TypeOf(arg)
	if t == nil {
		facts.problems = append(facts.problems, Unresolved{
			What:     label,
			Where:    a.prog.Pos(call.Pos(), a.root),
			Reason:   "request bind target has no resolved type",
			Category: "bind-target",
		})
		return true
	}
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		t = ptr.Elem()
	}
	if facts.request == nil {
		facts.request = t
		return true
	}
	if !types.Identical(facts.request, t) {
		facts.problems = append(facts.problems, Unresolved{
			What:     label,
			Where:    a.prog.Pos(call.Pos(), a.root),
			Reason:   "handler binds more than one distinct request body type",
			Category: "multi-request",
		})
	}
	return true
}

// matchResponse records a success response written by the handler. It reports
// whether the call was a response call. It is one dispatch table over the
// daemon's response helpers plus gin's own writers, kept together because it is
// the single place that maps a helper to a wire shape.
//
//nolint:gocyclo // See above: splitting the table would hide it.
func (a *analyzer) matchResponse(
	site *funcSite,
	call *ast.CallExpr,
	label string,
	facts *handlerFacts,
) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if fn, isFunc := site.Pkg.TypesInfo.Uses[sel.Sel].(*types.Func); isFunc &&
		fn.Pkg() != nil && fn.Pkg().Path() == a.responsePkg {
		switch fn.Name() {
		case "WriteQueryOK":
			a.addPayload(site, call, 1, label, facts)
			return true
		case "WriteQueryWithStatus":
			a.addPayload(site, call, 2, label, facts)
			return true
		case "WriteMutationOK":
			facts.kind = RespMutation
			return true
		case "WriteAccepted":
			a.setEmpty(facts)
			return true
		case "WriteErr":
			// The error envelope is emitted once, globally; it is not part
			// of any endpoint's success contract.
			return true
		}
		return false
	}
	if a.matchEncoderWrite(site, call, label, facts) {
		return true
	}
	if !isGinContextExpr(site, sel.X) {
		return false
	}
	switch sel.Sel.Name {
	case "JSON", "IndentedJSON", "PureJSON", "AsciiJSON", "SecureJSON":
		a.addPayload(site, call, 1, label, facts)
		return true
	case "Data", "File", "FileAttachment", "DataFromReader":
		facts.kind = RespBinary
		return true
	case "String":
		facts.kind = RespBinary
		return true
	case "Status":
		a.setEmpty(facts)
		return true
	}
	return false
}

// encodingJSONPath is the standard library's JSON package. A handler that
// streams through its Encoder is writing a body, not omitting one.
const encodingJSONPath = "encoding/json"

// matchEncoderWrite records a payload written straight into the response writer
// with `json.NewEncoder(w).Encode(v)`. It reports whether the call was one.
//
// This was found missing rather than designed in, and the way it was missing is
// the point: `GET /v0/.../review/outline` streams its envelope by hand — the
// one v0 payload large enough that gin's marshal-then-write would hold the
// whole 2.3 MB response in memory — and because it never touches `ctx.JSON` or
// a `libs.Write*` helper, the classifier fell through to `setEmpty`. The
// endpoint was reported as a **body-less success**, with no diagnostic, and its
// three DTOs (`outlineResponse`, `git.FileOutline`, `git.HunkShape`) were
// simply absent from the emitted crate. A silent drop is precisely what §9.2's
// "no silent drops" rule exists to prevent, and it stayed invisible because the
// wrong answer — `empty` — is indistinguishable from a correct one in every
// count the summary prints.
//
// Stated limit: this matches the Encoder by its package, not by proving its
// writer is the response. A handler that encoded JSON to something other than
// the client — a file, a log — would have that shape read as its response
// payload. Nothing in the daemon does, and the alternative (tracking the
// writer across the helper boundary, where it arrives as a bare `io.Writer`
// parameter) trades a stated limit for a fragile heuristic.
func (a *analyzer) matchEncoderWrite(
	site *funcSite,
	call *ast.CallExpr,
	label string,
	facts *handlerFacts,
) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Encode" {
		return false
	}
	fn, ok := site.Pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != encodingJSONPath {
		return false
	}
	a.addPayload(site, call, 0, label, facts)
	return true
}

// setEmpty records a body-less success response without clobbering a payload
// already found: handlers routinely call c.Status on a branch and write a
// payload on another.
func (a *analyzer) setEmpty(
	facts *handlerFacts,
) {
	if facts.kind == RespUnknown {
		facts.kind = RespEmpty
	}
}

// addPayload resolves the payload argument at index i of a response call.
func (a *analyzer) addPayload(
	site *funcSite,
	call *ast.CallExpr,
	i int,
	label string,
	facts *handlerFacts,
) {
	if i >= len(call.Args) {
		return
	}
	arg := unwrapEnvelopeLiteral(site, call.Args[i], a.res.envelopePath)
	t := site.Pkg.TypesInfo.TypeOf(arg)
	if t == nil {
		facts.problems = append(facts.problems, Unresolved{
			What:     label,
			Where:    a.prog.Pos(call.Pos(), a.root),
			Reason:   "response payload expression has no resolved type",
			Category: "payload-type",
		})
		return
	}
	facts.kind = RespJSON
	for _, existing := range facts.responses {
		if types.Identical(existing, t) {
			return
		}
	}
	facts.responses = append(facts.responses, t)
	facts.responseOrigins = append(facts.responseOrigins, a.prog.Pos(call.Pos(), a.root))
}

// unwrapEnvelopeLiteral rewrites `libs.Envelope{Data: x}` to `x`, so a handler
// that builds the envelope by hand still yields its payload type instead of
// the envelope's `any`.
func unwrapEnvelopeLiteral(
	site *funcSite,
	expr ast.Expr,
	envelopePath string,
) ast.Expr {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return expr
	}
	t := site.Pkg.TypesInfo.TypeOf(lit)
	named, ok := t.(*types.Named)
	if !ok || namedGoPath(named) != envelopePath {
		return expr
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if ok && key.Name == "Data" {
			return kv.Value
		}
	}
	return expr
}

// followHelper recurses into a call to a function declared in the handler's
// own package, which is where the daemon's bind/write helpers live.
func (a *analyzer) followHelper(
	site *funcSite,
	call *ast.CallExpr,
	label string,
	depth int,
	seen map[types.Object]bool,
	facts *handlerFacts,
) {
	obj := resolveFuncObject(site, call.Fun)
	if obj == nil || seen[obj] {
		return
	}
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != site.Pkg.PkgPath {
		return
	}
	callee := a.prog.FuncBody(obj)
	if callee == nil {
		return
	}
	seen[obj] = true
	a.walkBody(callee, label, depth+1, seen, facts)
}

// isGinContextExpr reports whether an expression denotes a *gin.Context.
func isGinContextExpr(
	site *funcSite,
	expr ast.Expr,
) bool {
	t := site.Pkg.TypesInfo.TypeOf(expr)
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && namedPath(named) == ginContext
}

// syntheticName mints a stable name for an anonymous Go struct that appears on
// the wire, derived from the handler that uses it. It shares the resolver's
// name registry so a synthetic name can never collide with a real Go type's.
func (a *analyzer) syntheticName(
	module string,
	base string,
) string {
	return a.res.declName(module, base)
}

// resolvePayload turns one payload type into a TypeRef, naming anonymous
// structs after the handler and flagging payloads that carry no static shape.
func (a *analyzer) resolvePayload(
	t types.Type,
	handlerPkg string,
	base string,
	label string,
	facts *handlerFacts,
) (TypeRef, bool) {
	if st, ok := t.(*types.Struct); ok {
		module := a.res.moduleFor(handlerPkg)
		name := a.syntheticName(module, base)
		key := module + "." + name
		decl := &Decl{
			Module:    module,
			Name:      name,
			GoPath:    handlerPkg + ".<anonymous " + base + ">",
			Kind:      KindStruct,
			Synthetic: true,
			Doc:       fmt.Sprintf("%s is the anonymous Go struct %s uses on the wire.", name, base),
		}
		a.res.decls[key] = decl
		decl.Fields, decl.Dropped = a.res.structFields(st, handlerPkg+"."+name, 0)
		return TypeRef{Module: module, Name: name}, true
	}
	if isUntypedPayload(t) {
		facts.problems = append(facts.problems, Unresolved{
			What:  label,
			Where: strings.Join(facts.responseOrigins, ", "),
			Reason: "response payload is an untyped map (gin.H / map[string]any), so it has no " +
				"static shape — give the handler a named DTO",
			Category: "untyped-payload",
		})
		ref, _ := a.res.Resolve(t, label)
		return ref, false
	}
	ref, ok := a.res.Resolve(t, label)
	if !ok {
		facts.problems = append(facts.problems, Unresolved{
			What:     label,
			Where:    strings.Join(facts.responseOrigins, ", "),
			Reason:   fmt.Sprintf("response payload type %s did not resolve", t.String()),
			Category: "payload-type",
		})
	}
	return ref, ok
}

// isUntypedPayload reports whether a payload type is an untyped string-keyed
// map — gin.H or map[string]any — whose fields cannot be recovered statically.
func isUntypedPayload(
	t types.Type,
) bool {
	m, ok := t.Underlying().(*types.Map)
	if !ok {
		return false
	}
	iface, ok := m.Elem().Underlying().(*types.Interface)
	return ok && iface.NumMethods() == 0
}

// dedupeRefs removes duplicate type references, preserving first-seen order.
func dedupeRefs(
	refs []TypeRef,
) []TypeRef {
	seen := map[string]bool{}
	out := []TypeRef{}
	for _, r := range refs {
		k := refKey(r)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// refKey renders a type reference as a comparable key.
func refKey(
	r TypeRef,
) string {
	var b strings.Builder
	b.WriteString(r.Container)
	b.WriteString("|")
	b.WriteString(r.Prim)
	b.WriteString("|")
	b.WriteString(r.Module)
	b.WriteString(".")
	b.WriteString(r.Name)
	if r.Key != nil {
		b.WriteString("<" + refKey(*r.Key))
	}
	if r.Elem != nil {
		b.WriteString(">" + refKey(*r.Elem))
	}
	return b.String()
}

// sortUnresolved orders a per-endpoint problem list deterministically.
func sortUnresolved(
	in []Unresolved,
) []Unresolved {
	out := make([]Unresolved, len(in))
	copy(out, in)
	for i := range out {
		if out[i].Severity == "" {
			out[i].Severity = SeverityError
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}
