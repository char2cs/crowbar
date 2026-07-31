package gen

import (
	"fmt"
	"go/types"
	"strings"
)

// Options configures one protogen run.
type Options struct {
	// DaemonRoot is the Go module root of the daemon (the api/ directory).
	DaemonRoot string
	// Patterns are the go/packages load patterns, relative to DaemonRoot.
	Patterns []string
	// EntryPkg is the import path of the package holding the router entry.
	EntryPkg string
	// EntryFunc is the name of the router entry function.
	EntryFunc string
	// MountPrefix is the path the entry router group is mounted at.
	MountPrefix string
	// ResponsePkg is the import path of the package holding the response
	// envelope and the Write* helpers every handler answers through.
	ResponsePkg string
}

// envelopePath is the fully qualified response envelope type.
func (o Options) envelopePath() string {
	return o.ResponsePkg + ".Envelope"
}

// DefaultOptions returns the options that describe the Crowbar daemon.
func DefaultOptions(
	daemonRoot string,
) Options {
	return Options{
		DaemonRoot:  daemonRoot,
		Patterns:    []string{"./internal/api/v0/..."},
		EntryPkg:    "github.com/char2cs/crowbar/api/internal/api/v0",
		EntryFunc:   "Register",
		MountPrefix: "/v0",
		ResponsePkg: "github.com/char2cs/crowbar/api/internal/api/libs",
	}
}

// Generate loads the daemon, discovers its routes, resolves every request and
// response DTO, and returns the IR plus everything that failed to resolve.
func Generate(
	opts Options,
) (*Result, error) {
	prog, err := Load(opts.DaemonRoot, opts.Patterns...)
	if err != nil {
		return nil, err
	}
	routes, err := prog.DiscoverRoutes(
		opts.EntryPkg,
		opts.EntryFunc,
		opts.MountPrefix,
		opts.DaemonRoot,
	)
	if err != nil {
		return nil, err
	}

	res := newResolver(prog, opts.DaemonRoot)
	res.envelopePath = opts.envelopePath()
	res.responsePkg = opts.ResponsePkg
	an := &analyzer{prog: prog, res: res, root: opts.DaemonRoot, responsePkg: opts.ResponsePkg}

	result := &Result{}
	for _, r := range routes {
		result.Endpoints = append(result.Endpoints, an.endpoint(r))
	}
	// The error envelope and the mutation payload are part of every surface,
	// so they are emitted unconditionally rather than only when some handler
	// happens to reference them.
	res.envelopeRef()
	res.mutationRef()

	result.Decls = res.Decls()
	markIncompleteEndpoints(result)
	result.Unresolved = append(result.Unresolved, res.unresolved...)
	for _, e := range result.Endpoints {
		result.Unresolved = append(result.Unresolved, e.Unresolved...)
	}
	result.sortResult()
	return result, nil
}

// markIncompleteEndpoints propagates dropped fields outward. A struct that lost
// a field is not the wire shape, and neither is anything that embeds it, so
// every endpoint whose request or response reaches one is reported unresolved
// rather than counted as covered.
func markIncompleteEndpoints(
	result *Result,
) {
	byKey := map[string]Decl{}
	for _, d := range result.Decls {
		byKey[d.Module+"."+d.Name] = d
	}
	incomplete := map[string]bool{}
	// Iterate to a fixed point: incompleteness flows from a struct to every
	// struct that references it, however deep the chain.
	for changed := true; changed; {
		changed = false
		for key, d := range byKey {
			if incomplete[key] {
				continue
			}
			if len(d.Dropped) > 0 || declReaches(d, incomplete) {
				incomplete[key] = true
				changed = true
			}
		}
	}

	for i := range result.Endpoints {
		e := &result.Endpoints[i]
		refs := append([]TypeRef{}, e.Responses...)
		if e.Request != nil {
			refs = append(refs, *e.Request)
		}
		for _, ref := range refs {
			for _, key := range refKeys(ref) {
				if !incomplete[key] {
					continue
				}
				e.Unresolved = append(e.Unresolved, Unresolved{
					What:     e.Method + " " + e.Path,
					Reason:   "DTO " + key + " is missing at least one field protogen could not lower, so this endpoint's shape is incomplete",
					Category: "incomplete-type",
					Severity: SeverityError,
				})
				break
			}
		}
		e.Unresolved = sortUnresolved(e.Unresolved)
	}
}

// declReaches reports whether any field of a declaration references a
// declaration already known to be incomplete.
func declReaches(
	d Decl,
	incomplete map[string]bool,
) bool {
	for _, f := range d.Fields {
		for _, key := range refKeys(f.Type) {
			if incomplete[key] {
				return true
			}
		}
	}
	if d.Underlying != nil {
		for _, key := range refKeys(*d.Underlying) {
			if incomplete[key] {
				return true
			}
		}
	}
	return false
}

// refKeys lists every declaration a type reference reaches.
func refKeys(
	ref TypeRef,
) []string {
	out := []string{}
	if ref.Name != "" {
		out = append(out, ref.Module+"."+ref.Name)
	}
	if ref.Key != nil {
		out = append(out, refKeys(*ref.Key)...)
	}
	if ref.Elem != nil {
		out = append(out, refKeys(*ref.Elem)...)
	}
	return out
}

// endpoint resolves one discovered route into a manifest entry.
func (a *analyzer) endpoint(
	r route,
) Endpoint {
	out := Endpoint{
		Method:       r.Method,
		Path:         r.Path,
		ResponseKind: RespUnknown,
	}
	if r.Handler == nil {
		out.ResponseKind = classifyHandlerlessRoute(r)
		if out.ResponseKind == RespUnknown {
			out.Unresolved = []Unresolved{{
				What:     r.Method + " " + r.Path,
				Where:    r.Site,
				Reason:   r.Reason,
				Category: "handler",
			}}
		}
		return out
	}

	site := a.prog.FuncBody(r.Handler)
	out.Handler = r.Handler.Name()
	out.HandlerPos = a.prog.Pos(r.Handler.Pos(), a.root)
	out.Doc = commentText(site.Decl.Doc)
	if r.Handler.Pkg() != nil {
		out.GoPackage = r.Handler.Pkg().Path()
	}

	label := r.Method + " " + r.Path
	facts := a.analyseHandler(site, label)
	out.ResponseKind = facts.kind

	if facts.request != nil {
		ref, ok := a.resolveRequest(facts.request, out, label, &facts)
		if ok {
			out.Request = &ref
		}
	}
	refs := []TypeRef{}
	for _, t := range facts.responses {
		ref, ok := a.resolvePayload(t, out.GoPackage, out.Handler+"Response", label, &facts)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	if facts.kind == RespMutation {
		refs = append(refs, a.res.mutationRef())
	}
	out.Responses = dedupeRefs(refs)
	out.Unresolved = sortUnresolved(facts.problems)
	return out
}

// classifyHandlerlessRoute recognises the routes that legitimately have no
// resolvable handler declaration: a WebSocket upgrade is an injected
// gin.HandlerFunc by construction, and carries no REST DTO.
func classifyHandlerlessRoute(
	r route,
) ResponseKind {
	if strings.Contains(r.Reason, "injected gin.HandlerFunc") {
		return RespWebSocket
	}
	return RespUnknown
}

// resolveRequest turns the bound request type into a reference, naming an
// anonymous request struct after its handler.
func (a *analyzer) resolveRequest(
	t types.Type,
	ep Endpoint,
	label string,
	facts *handlerFacts,
) (TypeRef, bool) {
	if st, ok := t.(*types.Struct); ok {
		module := a.res.moduleFor(ep.GoPackage)
		name := a.syntheticName(module, ep.Handler+"Request")
		key := module + "." + name
		decl := &Decl{
			Module:    module,
			Name:      name,
			GoPath:    ep.GoPackage + ".<anonymous " + ep.Handler + " request>",
			Kind:      KindStruct,
			Synthetic: true,
			Doc: fmt.Sprintf(
				"%s is the anonymous request body %s binds for %s.",
				name, ep.Handler, label,
			),
		}
		a.res.decls[key] = decl
		decl.Fields, decl.Dropped = a.res.structFields(st, ep.GoPackage+"."+name, 0)
		return TypeRef{Module: module, Name: name}, true
	}
	ref, ok := a.res.Resolve(t, label+" request")
	if !ok {
		facts.problems = append(facts.problems, Unresolved{
			What:     label,
			Reason:   fmt.Sprintf("request body type %s did not resolve", t.String()),
			Category: "request-type",
		})
	}
	return ref, ok
}
