package gen

import (
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
)

// Primitive tokens the emitters lower to their own syntax.
const (
	primString = "string"
	primBool   = "bool"
	primI8     = "i8"
	primI16    = "i16"
	primI32    = "i32"
	primI64    = "i64"
	primU8     = "u8"
	primU16    = "u16"
	primU32    = "u32"
	primU64    = "u64"
	primF32    = "f32"
	primF64    = "f64"
	// primJSON is an arbitrary JSON value: Go any, map[string]any, or
	// json.RawMessage.
	primJSON = "json"
	// primTime is a Go time.Time. encoding/json marshals it with
	// MarshalJSON, i.e. RFC 3339 with nanoseconds, as a JSON string.
	primTime = "time"
	// primBytes is a Go []byte. encoding/json base64-encodes it into a
	// JSON string.
	primBytes = "bytes"
)

// wellKnown maps fully qualified Go types that must not be structurally
// expanded, because encoding/json gives them a custom wire form.
var wellKnown = map[string]string{
	"time.Time":                primTime,
	"time.Duration":            primI64,
	"encoding/json.RawMessage": primJSON,
	"encoding/json.Number":     primString,
}

// The uniform response wrapper is emitted once, as a generic, instead of being
// expanded per payload type: its Data field is `any`, so structural expansion
// would lose the payload type entirely. Its import path comes from Options, so
// the tests can point protogen at a fixture package instead of the daemon.

// mutationDeclName is the name given to the daemon's fixed mutation payload,
// `data: {"id": "<uuid>"}`. The Go type behind it is unexported, so protogen
// names it.
const mutationDeclName = "MutationID"

// resolver turns go/types types into IR declarations, closing transitively.
type resolver struct {
	prog *Program
	root string
	// decls is the emitted set, keyed by "module.Name".
	decls map[string]*Decl
	// byGoPath maps a Go type path to the key in decls, so a second
	// reference reuses the first declaration.
	byGoPath map[string]string
	// modules maps a Go package path to its emitted module name.
	modules map[string]string
	// usedModules guards module-name collisions.
	usedModules map[string]string
	// inProgress breaks reference cycles during transitive closure.
	inProgress map[string]bool
	// usedNames guards declaration-name collisions inside a module, which
	// exporting an unexported Go type name can create.
	usedNames map[string]bool
	// declNames caches the emitted name of a Go type path.
	declNames map[string]string
	// envelopePath is the fully qualified response envelope type.
	envelopePath string
	// responsePkg is the import path of the response-helper package.
	responsePkg string
	// unresolved accumulates types protogen refused to expand.
	unresolved []Unresolved
	// unresolvedSeen dedupes the unresolved list.
	unresolvedSeen map[string]bool
}

func newResolver(
	prog *Program,
	root string,
) *resolver {
	return &resolver{
		prog:           prog,
		root:           root,
		decls:          map[string]*Decl{},
		byGoPath:       map[string]string{},
		modules:        map[string]string{},
		usedModules:    map[string]string{},
		inProgress:     map[string]bool{},
		usedNames:      map[string]bool{},
		declNames:      map[string]string{},
		unresolvedSeen: map[string]bool{},
	}
}

// note records an unresolved item once.
func (r *resolver) note(
	u Unresolved,
) {
	if u.Severity == "" {
		u.Severity = SeverityError
	}
	key := u.Category + "|" + u.What + "|" + u.Reason
	if r.unresolvedSeen[key] {
		return
	}
	r.unresolvedSeen[key] = true
	r.unresolved = append(r.unresolved, u)
}

// moduleFor derives a stable, unique emitted module name for a Go package.
func (r *resolver) moduleFor(
	pkgPath string,
) string {
	if m, ok := r.modules[pkgPath]; ok {
		return m
	}
	trimmed := pkgPath
	for _, prefix := range []string{
		"github.com/char2cs/crowbar/api/internal/",
		"github.com/char2cs/crowbar/api/",
		"github.com/char2cs/",
	} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			break
		}
	}
	if strings.Contains(trimmed, ".") {
		// Still a domain-qualified path: keep the last two segments only.
		segs := strings.Split(trimmed, "/")
		if len(segs) > 2 {
			segs = segs[len(segs)-2:]
		}
		trimmed = strings.Join(segs, "/")
	}
	name := sanitizeModule(trimmed)
	candidate := name
	for i := 2; ; i++ {
		owner, taken := r.usedModules[candidate]
		if !taken || owner == pkgPath {
			break
		}
		candidate = fmt.Sprintf("%s%d", name, i)
	}
	r.usedModules[candidate] = pkgPath
	r.modules[pkgPath] = candidate
	return candidate
}

// sanitizeModule reduces a package path fragment to a Rust/TS module name.
func sanitizeModule(
	s string,
) string {
	var b strings.Builder
	for _, ch := range s {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch + ('a' - 'A'))
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	out = strings.Trim(out, "_")
	if out == "" {
		return "pkg"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "m" + out
	}
	return out
}

// declName maps a Go type name onto the name emitted in both languages. Go
// unexported types (a handler's private request struct) still reach the wire,
// but a lowercase name is not a legal Rust type name by convention, so it is
// exported here — and deduped, in case the package also declares the exported
// spelling.
func (r *resolver) declName(
	module string,
	goName string,
) string {
	base := goName
	if base != "" {
		runes := []rune(base)
		if runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] = runes[0] - ('a' - 'A')
			base = string(runes)
		}
	}
	candidate := base
	for i := 2; r.usedNames[module+"."+candidate]; i++ {
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	r.usedNames[module+"."+candidate] = true
	return candidate
}

// nameFor returns the emitted name of a Go type, minting it once so a
// recursive type does not claim two names.
func (r *resolver) nameFor(
	module string,
	goPath string,
	goName string,
) string {
	if n, ok := r.declNames[goPath]; ok {
		return n
	}
	n := r.declName(module, goName)
	r.declNames[goPath] = n
	return n
}

// Resolve lowers a Go type into a TypeRef, emitting any declarations it needs.
// origin describes where the type came from, so failures name a real call site.
func (r *resolver) Resolve(
	t types.Type,
	origin string,
) (TypeRef, bool) {
	return r.resolve(t, origin, 0)
}

// resolve is one switch over the go/types type kinds. It stays a single
// function because it is the whole encoding/json lowering table; splitting it
// would scatter those rules across helpers with one caller each.
//
//nolint:gocyclo // See above.
func (r *resolver) resolve(
	t types.Type,
	origin string,
	depth int,
) (TypeRef, bool) {
	if depth > 32 {
		r.note(Unresolved{
			What:     origin,
			Reason:   "type nests deeper than 32 levels",
			Category: "type-depth",
		})
		return TypeRef{}, false
	}
	switch typ := t.(type) {
	case *types.Basic:
		return r.resolveBasic(typ, origin)
	case *types.Named:
		return r.resolveNamed(typ, origin, depth)
	case *types.Alias:
		return r.resolve(types.Unalias(typ), origin, depth)
	case *types.Pointer:
		elem, ok := r.resolve(typ.Elem(), origin, depth+1)
		if !ok {
			return TypeRef{}, false
		}
		return TypeRef{Container: "option", Elem: &elem}, true
	case *types.Slice:
		return r.resolveSlice(typ.Elem(), origin, depth)
	case *types.Array:
		return r.resolveSlice(typ.Elem(), origin, depth)
	case *types.Map:
		return r.resolveMap(typ, origin, depth)
	case *types.Interface:
		if typ.NumMethods() == 0 {
			return TypeRef{Prim: primJSON}, true
		}
		r.note(Unresolved{
			What:     origin,
			Reason:   "field is a non-empty interface; encoding/json emits whatever dynamic value it holds, so no static shape exists",
			Category: "interface",
		})
		return TypeRef{}, false
	case *types.Struct:
		r.note(Unresolved{
			What:     origin,
			Reason:   "anonymous struct nested inside another type; give it a named Go type so it can be generated",
			Category: "anonymous-struct",
		})
		return TypeRef{}, false
	case *types.Signature, *types.Chan:
		r.note(Unresolved{
			What:     origin,
			Reason:   "type is not JSON-serialisable (func or chan)",
			Category: "not-serialisable",
		})
		return TypeRef{}, false
	}
	r.note(Unresolved{
		What:     origin,
		Reason:   fmt.Sprintf("unsupported Go type %s", t.String()),
		Category: "unsupported-type",
	})
	return TypeRef{}, false
}

// resolveBasic maps a Go basic type to a primitive token.
func (r *resolver) resolveBasic(
	b *types.Basic,
	origin string,
) (TypeRef, bool) {
	switch b.Kind() {
	case types.Bool:
		return TypeRef{Prim: primBool}, true
	case types.String:
		return TypeRef{Prim: primString}, true
	case types.Int, types.Int64:
		return TypeRef{Prim: primI64}, true
	case types.Int8:
		return TypeRef{Prim: primI8}, true
	case types.Int16:
		return TypeRef{Prim: primI16}, true
	case types.Int32:
		return TypeRef{Prim: primI32}, true
	case types.Uint, types.Uint64, types.Uintptr:
		return TypeRef{Prim: primU64}, true
	case types.Uint8:
		return TypeRef{Prim: primU8}, true
	case types.Uint16:
		return TypeRef{Prim: primU16}, true
	case types.Uint32:
		return TypeRef{Prim: primU32}, true
	case types.Float32:
		return TypeRef{Prim: primF32}, true
	case types.Float64:
		return TypeRef{Prim: primF64}, true
	case types.UnsafePointer, types.Complex64, types.Complex128:
		r.note(Unresolved{
			What:     origin,
			Reason:   fmt.Sprintf("Go %s has no JSON encoding", b.String()),
			Category: "not-serialisable",
		})
		return TypeRef{}, false
	}
	r.note(Unresolved{
		What:     origin,
		Reason:   fmt.Sprintf("untyped or unhandled basic kind %s", b.String()),
		Category: "unsupported-type",
	})
	return TypeRef{}, false
}

// resolveSlice lowers []T, treating []byte as the base64 string encoding/json
// actually produces.
func (r *resolver) resolveSlice(
	elemType types.Type,
	origin string,
	depth int,
) (TypeRef, bool) {
	if b, ok := elemType.Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
		// []byte and every named byte slice: encoding/json base64-encodes it
		// into a JSON string rather than emitting an array of numbers.
		return TypeRef{Prim: primBytes}, true
	}
	elem, ok := r.resolve(elemType, origin, depth+1)
	if !ok {
		return TypeRef{}, false
	}
	return TypeRef{Container: "slice", Elem: &elem}, true
}

// resolveMap lowers map[K]V. encoding/json requires a string-like or integer
// key, so anything else is a hard failure worth reporting.
func (r *resolver) resolveMap(
	m *types.Map,
	origin string,
	depth int,
) (TypeRef, bool) {
	kb, ok := m.Key().Underlying().(*types.Basic)
	if !ok || (kb.Info()&types.IsString) == 0 && (kb.Info()&types.IsInteger) == 0 {
		r.note(Unresolved{
			What:     origin,
			Reason:   "map key is neither string-like nor integer, which encoding/json cannot marshal",
			Category: "map-key",
		})
		return TypeRef{}, false
	}
	key, ok := r.resolve(m.Key(), origin, depth+1)
	if !ok {
		return TypeRef{}, false
	}
	elem, ok := r.resolve(m.Elem(), origin, depth+1)
	if !ok {
		return TypeRef{}, false
	}
	return TypeRef{Container: "map", Key: &key, Elem: &elem}, true
}

// namedGoPath renders a named type as importpath.Name.
func namedGoPath(
	n *types.Named,
) string {
	obj := n.Obj()
	if obj == nil {
		return ""
	}
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Path() + "." + obj.Name()
}

// resolveNamed emits (or reuses) a declaration for a named Go type. The
// declaration is registered BEFORE its fields are walked, so a type that
// references itself resolves to the declaration already under construction
// instead of recursing until the depth guard trips.
func (r *resolver) resolveNamed(
	n *types.Named,
	origin string,
	depth int,
) (TypeRef, bool) {
	goPath := namedGoPath(n)
	if ref, done, ok := r.shortCircuitNamed(n, goPath, origin); done {
		return ref, ok
	}

	obj := n.Obj()
	module := r.moduleFor(obj.Pkg().Path())
	name := r.nameFor(module, goPath, obj.Name())
	key := module + "." + name
	if r.inProgress[key] {
		return TypeRef{Module: module, Name: name}, true
	}
	ref := TypeRef{Module: module, Name: name}

	switch under := n.Underlying().(type) {
	case *types.Struct:
		r.inProgress[key] = true
		defer delete(r.inProgress, key)
		decl := r.register(key, &Decl{
			Module: module,
			Name:   name,
			GoPath: goPath,
			Kind:   KindStruct,
			Doc:    r.prog.TypeDoc(obj),
		})
		decl.Fields, decl.Dropped = r.structFields(under, goPath, depth)
		return ref, true
	case *types.Basic:
		if under.Kind() == types.String {
			if variants := r.enumVariants(obj); len(variants) > 0 {
				r.register(key, &Decl{
					Module:   module,
					Name:     name,
					GoPath:   goPath,
					Kind:     KindEnum,
					Doc:      r.prog.TypeDoc(obj),
					Variants: variants,
				})
				return ref, true
			}
		}
		under2, ok := r.resolveBasic(under, origin)
		if !ok {
			return TypeRef{}, false
		}
		r.register(key, r.aliasDecl(module, name, goPath, obj, under2))
		return ref, true
	default:
		// A named slice, map or pointer type: emitted as an alias so the Go
		// name survives into both languages.
		r.inProgress[key] = true
		defer delete(r.inProgress, key)
		under2, ok := r.resolve(n.Underlying(), origin, depth+1)
		if !ok {
			return TypeRef{}, false
		}
		r.register(key, r.aliasDecl(module, name, goPath, obj, under2))
		return ref, true
	}
}

// shortCircuitNamed handles every named type that must NOT be expanded
// structurally. The second return reports whether it handled the type at all.
func (r *resolver) shortCircuitNamed(
	n *types.Named,
	goPath string,
	origin string,
) (TypeRef, bool, bool) {
	if prim, ok := wellKnown[goPath]; ok {
		return TypeRef{Prim: prim}, true, true
	}
	if goPath == r.envelopePath {
		return r.envelopeRef(), true, true
	}
	if key, ok := r.byGoPath[goPath]; ok {
		d := r.decls[key]
		return TypeRef{Module: d.Module, Name: d.Name}, true, true
	}
	if n.TypeParams() != nil && n.TypeParams().Len() > 0 {
		r.note(Unresolved{
			What:     origin,
			Where:    r.prog.Pos(n.Obj().Pos(), r.root),
			Reason:   fmt.Sprintf("generic Go type %s: protogen emits only the response envelope as a generic", goPath),
			Category: "generic-type",
		})
		return TypeRef{}, true, false
	}
	obj := n.Obj()
	if obj.Pkg() == nil {
		r.note(Unresolved{
			What:     origin,
			Reason:   fmt.Sprintf("named type %s has no package", goPath),
			Category: "unsupported-type",
		})
		return TypeRef{}, true, false
	}
	if hasCustomJSON(n) {
		// The struct shape is emitted anyway — refusing would drop whole
		// endpoints for a marshaller that only normalises nil slices — but the
		// generator cannot prove the marshaller is shape-preserving, so the type
		// is flagged for human confirmation rather than silently trusted.
		r.note(Unresolved{
			What:  goPath,
			Where: r.prog.Pos(obj.Pos(), r.root),
			Reason: fmt.Sprintf(
				"%s declares MarshalJSON/UnmarshalJSON: the emitted shape is its struct "+
					"shape, which the generator cannot prove matches the wire", goPath),
			Category: "custom-marshaller",
			Severity: SeverityWarning,
		})
	}
	return TypeRef{}, false, false
}

// register records a declaration under its key and its Go path, so the next
// reference to the same Go type reuses it.
func (r *resolver) register(
	key string,
	decl *Decl,
) *Decl {
	r.decls[key] = decl
	r.byGoPath[decl.GoPath] = key
	return decl
}

// aliasDecl builds the declaration for a named non-struct type.
func (r *resolver) aliasDecl(
	module string,
	name string,
	goPath string,
	obj types.Object,
	under TypeRef,
) *Decl {
	return &Decl{
		Module:     module,
		Name:       name,
		GoPath:     goPath,
		Kind:       KindAlias,
		Doc:        r.prog.TypeDoc(obj),
		Underlying: &under,
	}
}

// hasCustomJSON reports whether a named type declares MarshalJSON or
// UnmarshalJSON, which makes its struct shape a lie on the wire. Named.Method
// covers pointer-receiver methods too, which is where marshallers usually live.
func hasCustomJSON(
	n *types.Named,
) bool {
	for i := 0; i < n.NumMethods(); i++ {
		switch n.Method(i).Name() {
		case "MarshalJSON", "UnmarshalJSON":
			return true
		}
	}
	return false
}

// enumVariants returns the string constants declared for a named string type.
func (r *resolver) enumVariants(
	obj types.Object,
) []EnumVariant {
	consts := r.prog.EnumConsts(obj)
	if len(consts) == 0 {
		return nil
	}
	out := make([]EnumVariant, 0, len(consts))
	seen := map[string]bool{}
	for _, c := range consts {
		if seen[c.Value] {
			continue
		}
		seen[c.Value] = true
		out = append(out, EnumVariant{GoName: c.Name, Value: c.Value, Doc: c.Doc})
	}
	return out
}

// jsonField is one candidate field discovered while flattening a struct.
type jsonField struct {
	field Field
	depth int
	// tagged records whether the JSON name came from an explicit tag, which
	// is how encoding/json breaks same-depth name conflicts.
	tagged bool
	// index is the declaration order, used to keep output stable.
	index int
}

// structFields flattens a struct the way encoding/json does: embedded structs
// without a JSON tag have their fields promoted into the parent, shallower
// fields win name conflicts, and at equal depth exactly one tagged field wins.
//
// The second return is every field it had to LEAVE OUT. A dropped field is the
// one failure mode a generator must never hide: the struct would still compile
// and still deserialise, and the missing key would only surface as data that
// silently never arrives.
func (r *resolver) structFields(
	s *types.Struct,
	origin string,
	depth int,
) ([]Field, []Unresolved) {
	candidates := []jsonField{}
	dropped := []Unresolved{}
	r.collectFields(s, origin, depth, 0, &candidates, &dropped)

	byName := map[string][]jsonField{}
	order := []string{}
	for _, c := range candidates {
		if _, ok := byName[c.field.JSONName]; !ok {
			order = append(order, c.field.JSONName)
		}
		byName[c.field.JSONName] = append(byName[c.field.JSONName], c)
	}
	out := []Field{}
	kept := []jsonField{}
	for _, name := range order {
		winner, ok := pickField(byName[name])
		if !ok {
			u := Unresolved{
				What:     origin + "." + name,
				Reason:   "two embedded fields promote the same JSON name at the same depth; encoding/json drops both",
				Category: "field-conflict",
				Severity: SeverityError,
			}
			r.note(u)
			dropped = append(dropped, u)
			continue
		}
		kept = append(kept, winner)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].index < kept[j].index })
	for _, k := range kept {
		out = append(out, k.field)
	}
	return out, dropped
}

// pickField applies encoding/json's conflict rule to same-name candidates.
func pickField(
	cands []jsonField,
) (jsonField, bool) {
	if len(cands) == 1 {
		return cands[0], true
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.depth < best.depth {
			best = c
		}
	}
	sameDepth := []jsonField{}
	for _, c := range cands {
		if c.depth == best.depth {
			sameDepth = append(sameDepth, c)
		}
	}
	if len(sameDepth) == 1 {
		return sameDepth[0], true
	}
	tagged := []jsonField{}
	for _, c := range sameDepth {
		if c.tagged {
			tagged = append(tagged, c)
		}
	}
	if len(tagged) == 1 {
		return tagged[0], true
	}
	return jsonField{}, false
}

// collectFields walks one struct level, recursing into untagged embedded
// structs to promote their fields.
func (r *resolver) collectFields(
	s *types.Struct,
	origin string,
	depth int,
	embedDepth int,
	out *[]jsonField,
	dropped *[]Unresolved,
) {
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		tag := reflect.StructTag(s.Tag(i)).Get("json")
		name, opts := parseJSONTag(tag)
		if name == "-" && opts.raw == "" {
			continue
		}
		if f.Embedded() && name == "" {
			if r.promoteEmbedded(f, origin, depth, embedDepth, out, dropped) {
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		jsonName := name
		if jsonName == "" {
			jsonName = f.Name()
		}
		ref, ok := r.resolve(f.Type(), origin+"."+f.Name(), depth+1)
		if !ok {
			*dropped = append(*dropped, Unresolved{
				What:     origin + "." + f.Name(),
				Reason:   fmt.Sprintf("field type %s did not resolve, so the field is missing from the generated struct", f.Type().String()),
				Category: "dropped-field",
				Severity: SeverityError,
			})
			continue
		}
		*out = append(*out, jsonField{
			field: Field{
				JSONName: jsonName,
				GoName:   f.Name(),
				Type:     ref,
				Optional: fieldOptional(f.Type(), opts),
				Doc:      r.fieldDoc(f),
			},
			depth:  embedDepth,
			tagged: name != "",
			index:  len(*out),
		})
	}
}

// promoteEmbedded flattens an untagged embedded struct field. It reports
// whether the field was consumed by promotion.
func (r *resolver) promoteEmbedded(
	f *types.Var,
	origin string,
	depth int,
	embedDepth int,
	out *[]jsonField,
	dropped *[]Unresolved,
) bool {
	t := f.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	if hasCustomJSON(named) {
		// encoding/json promotes an embedded type's MarshalJSON to the outer
		// struct, which would replace the whole outer object with the inner
		// one's rendering. protogen keeps the field-promotion shape and flags
		// it rather than silently emitting a struct that cannot be right.
		r.note(Unresolved{
			What:  origin,
			Where: r.prog.Pos(named.Obj().Pos(), r.root),
			Reason: fmt.Sprintf(
				"embedded %s declares MarshalJSON, which encoding/json promotes to the "+
					"outer struct and applies to the whole object", namedGoPath(named)),
			Category: "embedded-marshaller",
		})
		return false
	}
	if _, ok := wellKnown[namedGoPath(named)]; ok {
		return false
	}
	inner, ok := named.Underlying().(*types.Struct)
	if !ok {
		// An embedded non-struct (e.g. a named string): encoding/json treats
		// it as a normal field named after its type.
		return false
	}
	r.collectFields(inner, origin, depth, embedDepth+1, out, dropped)
	return true
}

// fieldDoc returns the doc comment attached to a struct field, when the
// field's declaring package was loaded with syntax.
func (r *resolver) fieldDoc(
	f *types.Var,
) string {
	return r.prog.fieldDocs[f.Pos()]
}

// jsonOpts is the parsed option list of a json struct tag.
type jsonOpts struct {
	// omitEmpty is the classic omitempty option.
	omitEmpty bool
	// omitZero is Go 1.24's omitzero option.
	omitZero bool
	// asString is the ",string" option, which wraps the value in a JSON
	// string.
	asString bool
	// raw is the original option text after the name.
	raw string
}

// parseJSONTag splits a json struct tag into its name and options.
func parseJSONTag(
	tag string,
) (string, jsonOpts) {
	if tag == "" {
		return "", jsonOpts{}
	}
	name, rest, _ := strings.Cut(tag, ",")
	opts := jsonOpts{raw: rest}
	for _, o := range strings.Split(rest, ",") {
		switch o {
		case "omitempty":
			opts.omitEmpty = true
		case "omitzero":
			opts.omitZero = true
		case "string":
			opts.asString = true
		}
	}
	return name, opts
}

// fieldOptional decides whether a field can be missing or null on the wire.
// encoding/json's omitempty only fires for false, 0, a nil pointer/interface,
// and an empty string/array/slice/map — never for a struct — so a struct field
// tagged omitempty is still always present.
func fieldOptional(
	t types.Type,
	opts jsonOpts,
) bool {
	if _, isPtr := t.(*types.Pointer); isPtr {
		return true
	}
	if opts.omitZero {
		return true
	}
	if !opts.omitEmpty {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return true
	case *types.Slice, *types.Map, *types.Array:
		return true
	case *types.Interface:
		return true
	case *types.Pointer:
		return true
	case *types.Struct:
		return false
	default:
		_ = u
		return false
	}
}

// envelopeRef emits the generic response envelope declaration once and returns
// a reference to it.
func (r *resolver) envelopeRef() TypeRef {
	module := r.moduleFor(r.responsePkg)
	key := module + ".Envelope"
	if _, ok := r.decls[key]; !ok {
		r.decls[key] = &Decl{
			Module:  module,
			Name:    "Envelope",
			GoPath:  r.envelopePath,
			Kind:    KindStruct,
			Generic: true,
			Doc: "Envelope is the uniform response body returned by every v0 handler:\n" +
				"success is true for query and mutation responses and false for errors,\n" +
				"error carries the message on failure, and data carries the payload.",
			Fields: []Field{
				{JSONName: "success", GoName: "Success", Type: TypeRef{Prim: primBool}},
				{JSONName: "error", GoName: "Error", Type: TypeRef{Prim: primString}, Optional: true},
				{JSONName: "data", GoName: "Data", Type: TypeRef{Prim: "T"}, Optional: true},
			},
		}
		r.byGoPath[r.envelopePath] = key
	}
	return TypeRef{Module: module, Name: "Envelope"}
}

// mutationRef emits the fixed mutation payload declaration once and returns a
// reference to it.
func (r *resolver) mutationRef() TypeRef {
	module := r.moduleFor(r.responsePkg)
	key := module + "." + mutationDeclName
	if _, ok := r.decls[key]; !ok {
		r.decls[key] = &Decl{
			Module: module,
			Name:   mutationDeclName,
			GoPath: r.responsePkg + ".mutationData",
			Kind:   KindStruct,
			Doc: "MutationID is the fixed data shape of a mutation response:\n" +
				"the id of the affected entity.",
			Synthetic: true,
			Fields: []Field{
				{JSONName: "id", GoName: "ID", Type: TypeRef{Prim: primString}},
			},
		}
	}
	return TypeRef{Module: module, Name: mutationDeclName}
}

// Decls returns every emitted declaration.
func (r *resolver) Decls() []Decl {
	out := make([]Decl, 0, len(r.decls))
	for _, d := range r.decls {
		out = append(out, *d)
	}
	return out
}
