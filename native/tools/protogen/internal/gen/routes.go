package gen

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ginRouterGroup is the type whose method calls protogen interprets as route
// registrations, and whose values it tracks as path prefixes.
const ginRouterGroup = "github.com/gin-gonic/gin.RouterGroup"

// ginHandlerFunc is the handler type; the last argument of a route
// registration that has this type is the terminal handler.
const ginHandlerFunc = "github.com/gin-gonic/gin.HandlerFunc"

// httpMethods are the gin RouterGroup methods that register a route.
var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true,
}

// route is a discovered registration before its handler DTOs are resolved.
type route struct {
	// Method is the HTTP method.
	Method string
	// Path is the full path including every enclosing group prefix.
	Path string
	// Handler is the resolved terminal handler function, nil when the handler
	// expression could not be resolved to a declaration.
	Handler types.Object
	// HandlerExpr renders the unresolved handler expression, for the manifest.
	HandlerExpr string
	// Reason explains a nil Handler.
	Reason string
	// Site is the registration's source position.
	Site string
}

// routeWalker interprets the daemon's route-wiring code. gin builds paths at
// runtime by nesting RouterGroups, so a static extractor has to model that
// nesting: it tracks which local variables hold which group prefix, follows
// `x := g.Group("/p")` and cross-package `pkg.Register(group, ...)` calls, and
// records a route only at a concrete `g.METHOD("/rel", handlers...)` call.
type routeWalker struct {
	prog *Program
	root string
	// out accumulates discovered routes.
	out []route
	// seen guards against registration cycles between Register functions.
	seen map[types.Object]bool
	// depth bounds the recursion through Register functions.
	depth int
}

// DiscoverRoutes walks the router entry point and returns every registered
// (method, path, handler) triple. entryPkg is the package holding the entry
// function; entryFunc is its name (a method name resolves against any receiver
// in that package). prefix is the path the entry group is mounted at.
func (p *Program) DiscoverRoutes(
	entryPkgPath string,
	entryFunc string,
	prefix string,
	root string,
) ([]route, error) {
	pkg := p.Pkgs[entryPkgPath]
	if pkg == nil {
		return nil, fmt.Errorf("entry package %q not loaded", entryPkgPath)
	}
	site := p.findFunc(pkg, entryFunc)
	if site == nil {
		return nil, fmt.Errorf("entry func %q not found in %s", entryFunc, entryPkgPath)
	}
	w := &routeWalker{prog: p, root: root, seen: map[types.Object]bool{}}
	groups := map[types.Object]string{}
	bindGroupParams(site, []string{prefix}, groups)
	w.walkFunc(site, groups)
	return w.out, nil
}

// findFunc locates a top-level func or method declaration by name.
func (p *Program) findFunc(
	pkg *packages.Package,
	name string,
) *funcSite {
	for _, file := range pkg.Syntax {
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Name.Name != name {
				continue
			}
			obj := pkg.TypesInfo.Defs[fd.Name]
			if obj == nil {
				continue
			}
			return p.funcDecls[obj]
		}
	}
	return nil
}

// bindGroupParams binds the function's *gin.RouterGroup parameters to the
// given prefixes, positionally. A Register function that takes two groups (the
// terminal endpoint takes a workspace-scoped group and the top-level settings
// group) gets both bound from the call site.
func bindGroupParams(
	site *funcSite,
	prefixes []string,
	groups map[types.Object]string,
) {
	i := 0
	if site.Decl.Type.Params == nil {
		return
	}
	for _, field := range site.Decl.Type.Params.List {
		if !isRouterGroupExpr(site.Pkg, field.Type) {
			i += max(1, len(field.Names))
			continue
		}
		for _, name := range field.Names {
			if i < len(prefixes) {
				obj := site.Pkg.TypesInfo.Defs[name]
				if obj != nil {
					groups[obj] = prefixes[i]
				}
			}
			i++
		}
	}
}

// isRouterGroupExpr reports whether a parameter type expression denotes
// *gin.RouterGroup.
func isRouterGroupExpr(
	pkg *packages.Package,
	expr ast.Expr,
) bool {
	t := pkg.TypesInfo.TypeOf(expr)
	return isRouterGroup(t)
}

// isRouterGroup reports whether a type is *gin.RouterGroup.
func isRouterGroup(
	t types.Type,
) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	return namedPath(named) == ginRouterGroup
}

// namedPath renders a named type as importpath.Name.
func namedPath(
	n *types.Named,
) string {
	if n.Obj() == nil {
		return ""
	}
	if n.Obj().Pkg() == nil {
		return n.Obj().Name()
	}
	return n.Obj().Pkg().Path() + "." + n.Obj().Name()
}

// walkFunc interprets one function body against the group environment.
func (w *routeWalker) walkFunc(
	site *funcSite,
	groups map[types.Object]string,
) {
	if site.Decl.Body == nil {
		return
	}
	// A straight walk in source order is enough: route wiring is straight-line
	// code, and a Group() binding always precedes its uses.
	ast.Inspect(site.Decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			w.bindGroupAssign(site, node, groups)
		case *ast.CallExpr:
			w.visitCall(site, node, groups)
		}
		return true
	})
}

// bindGroupAssign records `x := <group>.Group("/p")` bindings.
func (w *routeWalker) bindGroupAssign(
	site *funcSite,
	assign *ast.AssignStmt,
	groups map[types.Object]string,
) {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return
	}
	base, ok := w.groupPrefix(site, sel.X, groups)
	if !ok {
		return
	}
	rel, ok := stringLit(site, call.Args, 0)
	if !ok {
		return
	}
	ident, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	obj := site.Pkg.TypesInfo.Defs[ident]
	if obj == nil {
		obj = site.Pkg.TypesInfo.Uses[ident]
	}
	if obj == nil {
		return
	}
	groups[obj] = joinPath(base, rel)
}

// groupPrefix resolves an expression that should denote a router group to its
// accumulated path prefix. It handles both a bound variable and an inline
// `parent.Group("/p")` chain.
func (w *routeWalker) groupPrefix(
	site *funcSite,
	expr ast.Expr,
	groups map[types.Object]string,
) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := site.Pkg.TypesInfo.Uses[e]
		if obj == nil {
			obj = site.Pkg.TypesInfo.Defs[e]
		}
		if obj == nil {
			return "", false
		}
		p, ok := groups[obj]
		return p, ok
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Group" {
			return "", false
		}
		base, ok := w.groupPrefix(site, sel.X, groups)
		if !ok {
			return "", false
		}
		rel, ok := stringLit(site, e.Args, 0)
		if !ok {
			return "", false
		}
		return joinPath(base, rel), true
	}
	return "", false
}

// visitCall handles the two call shapes that matter: a route registration on a
// tracked group, and a call into another package's Register with a tracked
// group as an argument.
func (w *routeWalker) visitCall(
	site *funcSite,
	call *ast.CallExpr,
	groups map[types.Object]string,
) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if httpMethods[sel.Sel.Name] {
		w.recordRoute(site, call, sel, groups)
		return
	}
	w.followRegister(site, call, groups)
}

// recordRoute turns one `g.METHOD("/rel", handlers...)` call into a route.
func (w *routeWalker) recordRoute(
	site *funcSite,
	call *ast.CallExpr,
	sel *ast.SelectorExpr,
	groups map[types.Object]string,
) {
	prefix, ok := w.groupPrefix(site, sel.X, groups)
	if !ok {
		return
	}
	rel, ok := stringLit(site, call.Args, 0)
	if !ok {
		w.out = append(w.out, route{
			Method:      sel.Sel.Name,
			Path:        prefix + "/<non-literal>",
			HandlerExpr: exprString(call),
			Reason:      "route path is not a string literal",
			Site:        w.prog.Pos(call.Pos(), w.root),
		})
		return
	}
	r := route{
		Method: sel.Sel.Name,
		Path:   joinPath(prefix, rel),
		Site:   w.prog.Pos(call.Pos(), w.root),
	}
	handlerExpr := terminalHandler(site, call.Args[1:])
	if handlerExpr == nil {
		r.Reason = "route registers no handler argument"
		w.out = append(w.out, r)
		return
	}
	r.HandlerExpr = exprString(handlerExpr)
	obj := resolveFuncObject(site, handlerExpr)
	if obj == nil {
		r.Reason = handlerReason(site, handlerExpr)
		w.out = append(w.out, r)
		return
	}
	if w.prog.FuncBody(obj) == nil {
		r.Reason = fmt.Sprintf("handler %s has no loaded source", obj.Name())
		w.out = append(w.out, r)
		return
	}
	r.Handler = obj
	w.out = append(w.out, r)
}

// terminalHandler picks the handler that actually answers the request out of a
// gin handler chain. The last argument is the terminal handler; when that last
// argument is a dual-serve dispatch call — `dispatch(rest, ws)`, the daemon's
// REST/WebSocket splitter — the REST half is its first argument.
func terminalHandler(
	site *funcSite,
	args []ast.Expr,
) ast.Expr {
	if len(args) == 0 {
		return nil
	}
	last := args[len(args)-1]
	call, ok := last.(*ast.CallExpr)
	if !ok {
		return last
	}
	if !isDispatchSignature(site.Pkg.TypesInfo.TypeOf(call.Fun)) || len(call.Args) == 0 {
		return last
	}
	return call.Args[0]
}

// isDispatchSignature reports whether a type is
// func(gin.HandlerFunc, gin.HandlerFunc) gin.HandlerFunc — the dual-serve
// wrapper's shape.
func isDispatchSignature(
	t types.Type,
) bool {
	sig, ok := t.(*types.Signature)
	if !ok || sig.Params().Len() != 2 || sig.Results().Len() != 1 {
		return false
	}
	isHF := func(t types.Type) bool {
		named, ok := t.(*types.Named)
		return ok && namedPath(named) == ginHandlerFunc
	}
	return isHF(sig.Params().At(0).Type()) &&
		isHF(sig.Params().At(1).Type()) &&
		isHF(sig.Results().At(0).Type())
}

// resolveFuncObject resolves a handler expression to the function object it
// names. It handles a method value (h.Status), a package-level func
// (pkg.Handle), and a plain identifier bound to a func declaration.
func resolveFuncObject(
	site *funcSite,
	expr ast.Expr,
) types.Object {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if obj, ok := site.Pkg.TypesInfo.Uses[e.Sel].(*types.Func); ok {
			return obj
		}
	case *ast.Ident:
		if obj, ok := site.Pkg.TypesInfo.Uses[e].(*types.Func); ok {
			return obj
		}
	}
	return nil
}

// handlerReason explains why a handler expression did not resolve, in terms of
// what the Go side would have to change.
func handlerReason(
	site *funcSite,
	expr ast.Expr,
) string {
	t := site.Pkg.TypesInfo.TypeOf(expr)
	if named, ok := t.(*types.Named); ok && namedPath(named) == ginHandlerFunc {
		return "handler is an injected gin.HandlerFunc value (WebSocket upgrade or " +
			"closure), not a declared function"
	}
	return fmt.Sprintf("handler expression %s does not resolve to a declared function",
		exprString(expr))
}

// followRegister recurses into a `pkg.Register(group, ...)` call, binding the
// callee's router-group parameters to the prefixes of the groups passed in.
func (w *routeWalker) followRegister(
	site *funcSite,
	call *ast.CallExpr,
	groups map[types.Object]string,
) {
	if w.depth > 8 {
		return
	}
	prefixes := []string{}
	any := false
	for _, arg := range call.Args {
		if !isRouterGroup(site.Pkg.TypesInfo.TypeOf(arg)) {
			continue
		}
		p, ok := w.groupPrefix(site, arg, groups)
		if !ok {
			return
		}
		prefixes = append(prefixes, p)
		any = true
	}
	if !any {
		return
	}
	obj := resolveFuncObject(site, call.Fun)
	if obj == nil {
		return
	}
	callee := w.prog.FuncBody(obj)
	if callee == nil || w.seen[obj] {
		return
	}
	w.seen[obj] = true
	defer delete(w.seen, obj)

	sub := map[types.Object]string{}
	bindGroupParams(callee, prefixes, sub)
	w.depth++
	w.walkFunc(callee, sub)
	w.depth--
}

// stringLit extracts a string literal (or a resolved string constant) from an
// argument list position.
func stringLit(
	site *funcSite,
	args []ast.Expr,
	i int,
) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	if tv, ok := site.Pkg.TypesInfo.Types[args[i]]; ok && tv.Value != nil {
		return constValueString(tv.Value.String()), true
	}
	return "", false
}

// constValueString unquotes a go/constant string rendering.
func constValueString(
	s string,
) string {
	if unquoted, err := unquoteGo(s); err == nil {
		return unquoted
	}
	return strings.Trim(s, `"`)
}

// exprString renders an expression compactly for diagnostics.
func exprString(
	expr ast.Expr,
) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		parts := make([]string, 0, len(e.Args))
		for _, a := range e.Args {
			parts = append(parts, exprString(a))
		}
		return exprString(e.Fun) + "(" + strings.Join(parts, ", ") + ")"
	case *ast.UnaryExpr:
		return e.Op.String() + exprString(e.X)
	case *ast.BasicLit:
		return e.Value
	case *ast.CompositeLit:
		if e.Type != nil {
			return exprString(e.Type) + "{…}"
		}
		return "{…}"
	case *ast.IndexExpr:
		return exprString(e.X) + "[…]"
	case *ast.StarExpr:
		return "*" + exprString(e.X)
	}
	return "<expr>"
}
