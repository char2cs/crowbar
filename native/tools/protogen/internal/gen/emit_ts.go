package gen

import (
	"fmt"
	"sort"
	"strings"
)

// tsPrims maps IR primitives to TypeScript types.
var tsPrims = map[string]string{
	primString: "string",
	primBool:   "boolean",
	primI8:     "number",
	primI16:    "number",
	primI32:    "number",
	primI64:    "number",
	primU8:     "number",
	primU16:    "number",
	primU32:    "number",
	primU64:    "number",
	primF32:    "number",
	primF64:    "number",
	primJSON:   "unknown",
	// encoding/json renders time.Time as an RFC 3339 string.
	primTime: "string",
	// encoding/json base64-encodes []byte into a string.
	primBytes: "string",
}

// EmitTS renders every module as a TypeScript file plus an index barrel,
// keyed by file name.
func EmitTS(
	r *Result,
) map[string]string {
	files := map[string]string{}
	modules := r.Modules()
	aliases := nullableAliases(r)
	for _, m := range modules {
		files[m+".ts"] = tsModule(m, r.DeclsFor(m), aliases)
	}

	var b strings.Builder
	b.WriteString(tsGeneratedHeader + "\n")
	b.WriteString("//\n")
	b.WriteString("// Wire DTOs for the Crowbar daemon's v0 HTTP surface, generated from the Go\n")
	b.WriteString("// handlers. One namespace per Go package, so type names from different\n")
	b.WriteString("// packages can never collide.\n")
	b.WriteString("//\n")
	b.WriteString("// Encoding notes, all of which mirror what encoding/json actually does:\n")
	b.WriteString("//   * time.Time is a string (RFC 3339, as time.Time's MarshalJSON writes it).\n")
	b.WriteString("//   * int64/uint64 are numbers, NOT strings: the daemon uses no `,string`\n")
	b.WriteString("//     tags, so encoding/json emits them as JSON numbers. Values beyond\n")
	b.WriteString("//     2^53 lose precision in JavaScript; none of the daemon's int64 fields\n")
	b.WriteString("//     carry such values today.\n")
	b.WriteString("//   * []byte is a string (encoding/json base64-encodes it).\n")
	b.WriteString("//   * A nil Go slice or map marshals as null, so non-optional container\n")
	b.WriteString("//     fields are typed `T[] | null`.\n")
	b.WriteString("\n")
	for _, m := range modules {
		b.WriteString(fmt.Sprintf("export * as %s from %q;\n", m, "./"+m))
	}
	files["index.ts"] = b.String()
	return files
}

// tsModule renders one module file with the imports its references need.
func tsModule(
	module string,
	decls []Decl,
	aliases map[string]bool,
) string {
	imports := map[string]bool{}
	for _, d := range decls {
		for _, f := range d.Fields {
			collectModules(f.Type, module, imports)
		}
		if d.Underlying != nil {
			collectModules(*d.Underlying, module, imports)
		}
	}
	names := make([]string, 0, len(imports))
	for m := range imports {
		names = append(names, m)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(tsGeneratedHeader + "\n")
	b.WriteString("\n")
	for _, m := range names {
		b.WriteString(fmt.Sprintf("import type * as %s from %q;\n", m, "./"+m))
	}
	if len(names) > 0 {
		b.WriteString("\n")
	}
	for i, d := range decls {
		if i > 0 {
			b.WriteString("\n")
		}
		switch d.Kind {
		case KindStruct:
			b.WriteString(tsStruct(d, module, aliases))
		case KindEnum:
			b.WriteString(tsEnum(d))
		case KindAlias:
			b.WriteString(tsAlias(d, module))
		}
	}
	return b.String()
}

// collectModules records every foreign module a reference reaches.
func collectModules(
	ref TypeRef,
	self string,
	out map[string]bool,
) {
	if ref.Module != "" && ref.Module != self {
		out[ref.Module] = true
	}
	if ref.Key != nil {
		collectModules(*ref.Key, self, out)
	}
	if ref.Elem != nil {
		collectModules(*ref.Elem, self, out)
	}
}

// tsDoc renders a JSDoc block at the given indent.
func tsDoc(
	doc string,
	indent string,
) string {
	lines := docLines(doc)
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(indent + "/**\n")
	for _, line := range lines {
		b.WriteString(indent + " *")
		if line != "" {
			b.WriteString(" " + strings.ReplaceAll(line, "*/", "*\\/"))
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + " */\n")
	return b.String()
}

// tsDroppedNote writes an INCOMPLETE banner onto an interface that lost a
// field, for the same reason the Rust emitter does: a missing key is otherwise
// indistinguishable from a key that was never there.
func tsDroppedNote(
	d Decl,
) string {
	if len(d.Dropped) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("/**\n")
	b.WriteString(" * INCOMPLETE: protogen could not lower every field of this type.\n")
	for _, u := range d.Dropped {
		b.WriteString(" * - " + u.What + ": " + u.Reason + "\n")
	}
	b.WriteString(" */\n")
	return b.String()
}

// tsStruct renders an interface declaration.
func tsStruct(
	d Decl,
	self string,
	aliases map[string]bool,
) string {
	var b strings.Builder
	b.WriteString(tsDoc(d.Doc, ""))
	b.WriteString(tsDroppedNote(d))
	generics := ""
	if d.Generic {
		generics = "<T>"
	}
	b.WriteString("export interface " + d.Name + generics + " {\n")
	for _, f := range d.Fields {
		b.WriteString(tsDoc(f.Doc, "  "))
		key := f.JSONName
		if !isTSIdent(key) {
			key = fmt.Sprintf("%q", key)
		}
		opt := ""
		if f.Optional {
			opt = "?"
		}
		ty := tsType(f.Type, self)
		if !f.Optional && refNullable(f.Type, aliases) {
			ty += " | null"
		}
		b.WriteString("  " + key + opt + ": " + ty + ";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// tsType renders a type reference.
func tsType(
	ref TypeRef,
	self string,
) string {
	switch ref.Container {
	case "option":
		return tsType(*ref.Elem, self) + " | null"
	case "slice":
		inner := tsType(*ref.Elem, self)
		if strings.ContainsAny(inner, "|&") {
			return "Array<" + inner + ">"
		}
		return inner + "[]"
	case "map":
		return "Record<" + tsMapKey(*ref.Key) + ", " + tsType(*ref.Elem, self) + ">"
	}
	if ref.Name != "" {
		if ref.Module == self {
			return ref.Name
		}
		return ref.Module + "." + ref.Name
	}
	if ref.Prim == "T" {
		return "T"
	}
	if p, ok := tsPrims[ref.Prim]; ok {
		return p
	}
	return "unknown"
}

// tsMapKey renders a map key type: JSON object keys are always strings, but a
// named string enum key stays typed so the mapping is not widened.
func tsMapKey(
	ref TypeRef,
) string {
	if ref.Name != "" {
		return "string"
	}
	if ref.Prim == primString {
		return "string"
	}
	return "string"
}

// tsEnum renders a Go string-constant set as a union type. The trailing
// `(string & {})` member mirrors the Rust Other(String) fallback: a Go named
// string type is an open set, and its zero value "" is never a declared
// constant. The intersection keeps editor completion on the known members.
func tsEnum(
	d Decl,
) string {
	var b strings.Builder
	b.WriteString(tsDoc(d.Doc, ""))
	b.WriteString("export type " + d.Name + " =\n")
	for _, v := range d.Variants {
		b.WriteString(fmt.Sprintf("  | %q\n", v.Value))
	}
	b.WriteString("  | (string & {});\n")
	return b.String()
}

// tsAlias renders a named Go type whose underlying type is not a struct.
func tsAlias(
	d Decl,
	self string,
) string {
	var b strings.Builder
	b.WriteString(tsDoc(d.Doc, ""))
	b.WriteString("export type " + d.Name + " = " + tsType(*d.Underlying, self) + ";\n")
	return b.String()
}
