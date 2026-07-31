package gen

import (
	"fmt"
	"sort"
	"strings"
)

// RustTestsFile is the file name EmitRustTests writes into the -out-rust-tests
// directory.
const RustTestsFile = "generated_roundtrip.rs"

// maxSampleDepth bounds the sample walk. Go's type system already forbids a
// struct that contains itself without indirection, so the cycle breaker below
// is what actually terminates recursion; this is the belt to its braces, and it
// fails loudly rather than hanging if the IR ever grows a shape Go could not
// have produced.
const maxSampleDepth = 48

// EmitRustTests renders the round-trip suite for the emitted Rust DTOs, keyed
// by file name, plus the declarations it refused to cover and why.
//
// Why generate the tests rather than hand-write them: the thing under test is
// itself generated, so a hand-written suite drifts silently the first time a Go
// handler grows a field — the new field would arrive untested and nothing would
// say so. Deriving both from the same IR means a wire shape cannot change
// without its test changing in the same commit.
//
// What each emitted test actually proves, in the order it proves it:
//
//  1. A fixture written in the *daemon's* JSON key names deserialises to a
//     specific Rust value. That is what pins `rename_all = "camelCase"` and
//     every explicit `rename` — a round trip alone would not, because a struct
//     that renames a field wrongly still round-trips through itself perfectly.
//  2. Serialising that value reproduces the fixture's wire shape.
//  3. serialize → deserialize → compare, the round trip proper.
//  4. The zero-value fixture: absent optional fields, and `null` where Go
//     marshals a nil slice or map. This is the only thing that exercises the
//     generated `null_to_default` helper, and it asserts the *normalised* form
//     comes back out (`[]`, not `null`), so the coercion is proved to happen
//     rather than merely be tolerated.
//  5. For enums: every declared constant by its exact wire string, both
//     directions; the open-set fallback on `""` and on an undeclared value;
//     and — the assertion that would actually fail if the untagged `Other`
//     variant were ordered before the named ones — that a declared value does
//     *not* land in `Other`.
func EmitRustTests(
	r *Result,
) (map[string]string, []Unresolved) {
	s := &sampler{byKey: map[string]Decl{}, aliases: nullableAliases(r)}
	for _, d := range r.Decls {
		s.byKey[d.Module+"."+d.Name] = d
	}

	var (
		bodies  []string
		skipped []Unresolved
		covered int
	)
	for _, d := range r.Decls {
		body, err := s.declTests(d)
		if err != nil {
			skipped = append(skipped, Unresolved{
				What:     d.GoPath,
				Reason:   err.Error(),
				Category: "untestable-type",
				Severity: SeverityWarning,
			})
			continue
		}
		if body == "" {
			continue
		}
		covered++
		bodies = append(bodies, body)
	}

	var b strings.Builder
	b.WriteString(GeneratedHeader + "\n")
	b.WriteString(rustTestsPreamble(covered, len(r.Decls), skipped))
	for i, body := range bodies {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(body)
	}
	return map[string]string{RustTestsFile: b.String()}, skipped
}

// rustTestsPreamble renders the header comment and the crate-level allows.
func rustTestsPreamble(
	covered int,
	total int,
	skipped []Unresolved,
) string {
	var b strings.Builder
	b.WriteString("//\n")
	b.WriteString("// Round-trip tests for the generated wire DTOs. Generated from the same IR as\n")
	b.WriteString("// the DTOs themselves, so a Go handler cannot grow, lose or rename a field\n")
	b.WriteString("// without this file changing in the same commit.\n")
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// %d of %d generated declarations are covered here.\n", covered, total)
	if len(skipped) > 0 {
		b.WriteString("//\n")
		b.WriteString("// NOT covered, because no value of the type can be written without a\n")
		b.WriteString("// dependency this crate does not have (spec §4.2 allows serde and nothing\n")
		b.WriteString("// else). These declarations are not emitted into the crate either:\n")
		for _, u := range skipped {
			b.WriteString("//   * " + u.What + " — " + u.Reason + "\n")
		}
	}
	b.WriteString("//\n")
	b.WriteString("// The blanket allow is deliberate and is not hiding anything: this is\n")
	b.WriteString("// machine-written test code with a machine's taste in naming and literals,\n")
	b.WriteString("// and `clippy::pedantic` is denied workspace-wide (§4.3 rule 4). Style lints\n")
	b.WriteString("// here would only ever be answered by editing the generator's whitespace.\n")
	b.WriteString("#![allow(clippy::all, clippy::pedantic, non_snake_case)]\n")
	b.WriteString("\n")
	return b.String()
}

// sample is one generated value: the Rust expression that builds it, the JSON a
// fixture feeds in, and the JSON serialising it produces. In and out differ
// exactly where the wire is lossy — a nil Go container arrives as `null` and
// leaves as `[]` — which is the behaviour worth pinning.
type sample struct {
	rust    string
	jsonIn  string
	jsonOut string
}

// omitted marks a field that is absent from the wire entirely.
const omitted = "\x00omitted"

// sampler builds sample values from the IR.
type sampler struct {
	byKey   map[string]Decl
	aliases map[string]bool
	n       int
	stack   map[string]bool
	depth   int
}

// next returns the counter that keeps every scalar in one sample distinct, so
// that two fields swapped by a bad rename cannot both still match.
func (s *sampler) next() int {
	s.n++
	return s.n
}

// declTests renders the tests for one declaration, or "" when the declaration
// needs none.
func (s *sampler) declTests(
	d Decl,
) (string, error) {
	switch d.Kind {
	case KindStruct:
		return s.structTests(d)
	case KindEnum:
		return s.enumTests(d), nil
	case KindAlias:
		return s.aliasTests(d)
	}
	return "", nil
}

// rustPath is the fully qualified path to a declaration from the test crate.
// Fully qualified rather than imported because names repeat across modules —
// `Range` and `Position` each exist more than once — and an import list would
// need per-name aliasing to stay unambiguous.
func rustPath(
	d Decl,
) string {
	return "crowbar_proto::" + d.Module + "::" + d.Name
}

// testName is the test function name for one declaration and one case.
func testName(
	d Decl,
	suffix string,
) string {
	return d.Module + "_" + toSnake(d.Name) + "_" + suffix
}

// structTests renders the populated and zero-value cases for a struct.
func (s *sampler) structTests(
	d Decl,
) (string, error) {
	full, err := s.fresh(func() (sample, error) { return s.structValue(d, false) })
	if err != nil {
		return "", err
	}
	empty, err := s.fresh(func() (sample, error) { return s.structValue(d, true) })
	if err != nil {
		return "", err
	}

	ty := rustPath(d)
	if d.Generic {
		ty += "<String>"
	}

	var b strings.Builder
	b.WriteString("/// The wire shape of `" + d.GoPath + "`, fully populated.\n")
	b.WriteString("#[test]\n")
	b.WriteString("fn " + testName(d, "wire_shape") + "() {\n")
	b.WriteString("    let value: " + ty + " = " + full.rust + ";\n")
	b.WriteString("    let wire = " + rustRawStr(full.jsonIn) + ";\n")
	b.WriteString("\n")
	b.WriteString("    // The daemon's key names map onto these fields, and onto no others.\n")
	b.WriteString("    let parsed: " + ty + " =\n")
	b.WriteString("        serde_json::from_str(wire).expect(\"deserialise the populated fixture\");\n")
	b.WriteString("    assert_eq!(parsed, value);\n")
	b.WriteString("\n")
	b.WriteString("    // ... and serialising reproduces that same shape, key for key.\n")
	b.WriteString("    let produced = serde_json::to_value(&value).expect(\"serialise\");\n")
	b.WriteString(expectedShape(full))
	b.WriteString("    assert_eq!(produced, expected);\n")
	b.WriteString("\n")
	b.WriteString("    // serialize -> deserialize -> compare.\n")
	b.WriteString("    let text = serde_json::to_string(&value).expect(\"serialise\");\n")
	b.WriteString("    let back: " + ty + " = serde_json::from_str(&text).expect(\"round trip\");\n")
	b.WriteString("    assert_eq!(back, value);\n")
	b.WriteString("\n")
	b.WriteString("    // The rest of the derive set.\n")
	b.WriteString("    assert_eq!(value.clone(), value);\n")
	b.WriteString("    assert!(!format!(\"{value:?}\").is_empty());\n")
	b.WriteString("}\n")

	b.WriteString("\n")
	b.WriteString("/// `" + d.GoPath + "` at Go's zero values: every optional field absent, and\n")
	b.WriteString("/// `null` wherever a nil Go slice or map would marshal to one.\n")
	b.WriteString("#[test]\n")
	b.WriteString("fn " + testName(d, "zero_values") + "() {\n")
	b.WriteString("    let value: " + ty + " = " + empty.rust + ";\n")
	b.WriteString("    let wire = " + rustRawStr(empty.jsonIn) + ";\n")
	b.WriteString("\n")
	b.WriteString("    let parsed: " + ty + " =\n")
	b.WriteString("        serde_json::from_str(wire).expect(\"deserialise the zero-value fixture\");\n")
	b.WriteString("    assert_eq!(parsed, value);\n")
	b.WriteString("\n")
	b.WriteString("    // Serialising normalises: an absent optional stays absent, and a null\n")
	b.WriteString("    // container comes back out as an empty one rather than as null.\n")
	b.WriteString("    let produced = serde_json::to_value(&value).expect(\"serialise\");\n")
	b.WriteString(expectedShape(empty))
	b.WriteString("    assert_eq!(produced, expected);\n")
	b.WriteString("\n")
	b.WriteString("    let text = serde_json::to_string(&value).expect(\"serialise\");\n")
	b.WriteString("    let back: " + ty + " = serde_json::from_str(&text).expect(\"round trip\");\n")
	b.WriteString("    assert_eq!(back, value);\n")
	b.WriteString("}\n")
	return b.String(), nil
}

// enumTests renders the per-variant table plus the open-set edge cases.
func (s *sampler) enumTests(
	d Decl,
) string {
	ty := rustPath(d)
	names := variantNames(d)

	var b strings.Builder
	b.WriteString("/// Every constant of `" + d.GoPath + "`, by its exact wire value, plus the\n")
	b.WriteString("/// open-set fallback a Go named string type requires.\n")
	b.WriteString("#[test]\n")
	b.WriteString("fn " + testName(d, "variants") + "() {\n")
	if len(d.Variants) > 0 {
		fmt.Fprintf(&b, "    let cases: [(&str, %s); %d] = [\n", ty, len(d.Variants))
		for i, v := range d.Variants {
			b.WriteString("        (" + rustStr(v.Value) + ", " + ty + "::" + names[i] + "),\n")
		}
		b.WriteString("    ];\n")
		b.WriteString("    for (wire, variant) in cases {\n")
		b.WriteString("        let json = serde_json::to_string(wire).expect(\"quote the wire value\");\n")
		b.WriteString("        let parsed: " + ty + " =\n")
		b.WriteString("            serde_json::from_str(&json).expect(\"deserialise a declared constant\");\n")
		b.WriteString("        assert_eq!(parsed, variant);\n")
		b.WriteString("        assert_eq!(serde_json::to_string(&variant).expect(\"serialise\"), json);\n")
		b.WriteString("\n")
		b.WriteString("        // A declared value must not fall through to the untagged variant.\n")
		b.WriteString("        // This is the assertion that fails if `Other` is ever ordered\n")
		b.WriteString("        // before the named variants: untagged matching is first-wins.\n")
		b.WriteString("        assert_ne!(parsed, " + ty + "::Other(wire.to_owned()));\n")
		b.WriteString("\n")
		b.WriteString("        assert_eq!(variant.clone(), variant);\n")
		b.WriteString("        assert!(!format!(\"{variant:?}\").is_empty());\n")
		b.WriteString("    }\n")
		b.WriteString("\n")
	}
	if !declaresValue(d, "") {
		b.WriteString("    // Go's zero value is a legal value that no constant names, which is\n")
		b.WriteString("    // the whole reason the untagged variant exists.\n")
		b.WriteString("    let zero: " + ty + " = serde_json::from_str(\"\\\"\\\"\").expect(\"deserialise the zero value\");\n")
		b.WriteString("    assert_eq!(zero, " + ty + "::Other(String::new()));\n")
		b.WriteString("    assert_eq!(serde_json::to_string(&zero).expect(\"serialise\"), \"\\\"\\\"\");\n")
		b.WriteString("\n")
	}
	unknown := "a-value-no-constant-declares"
	b.WriteString("    // And a value added on the Go side after this was generated must widen\n")
	b.WriteString("    // the set rather than fail the whole response.\n")
	b.WriteString("    let unknown: " + ty + " =\n")
	b.WriteString("        serde_json::from_str(" + rustStr(`"`+unknown+`"`) + ").expect(\"deserialise an undeclared value\");\n")
	b.WriteString("    assert_eq!(unknown, " + ty + "::Other(" + rustStr(unknown) + ".to_owned()));\n")
	b.WriteString("    assert_eq!(\n")
	b.WriteString("        serde_json::to_string(&unknown).expect(\"serialise\"),\n")
	b.WriteString("        " + rustStr(`"`+unknown+`"`) + ",\n")
	b.WriteString("    );\n")
	b.WriteString("    assert_eq!(unknown.clone(), unknown);\n")
	b.WriteString("    assert!(!format!(\"{unknown:?}\").is_empty());\n")
	b.WriteString("}\n")
	return b.String()
}

// aliasTests renders a round trip through a named non-struct type.
func (s *sampler) aliasTests(
	d Decl,
) (string, error) {
	v, err := s.fresh(func() (sample, error) { return s.value(*d.Underlying, false) })
	if err != nil {
		return "", err
	}
	ty := rustPath(d)
	var b strings.Builder
	b.WriteString("/// `" + d.GoPath + "` is a named Go type with no struct shape; it is an alias\n")
	b.WriteString("/// on the Rust side and must still carry its underlying type's wire form.\n")
	b.WriteString("#[test]\n")
	b.WriteString("fn " + testName(d, "alias_round_trip") + "() {\n")
	b.WriteString("    let value: " + ty + " = " + v.rust + ";\n")
	b.WriteString("    let text = serde_json::to_string(&value).expect(\"serialise\");\n")
	b.WriteString("    let expected: serde_json::Value =\n")
	b.WriteString("        serde_json::from_str(" + rustRawStr(v.jsonOut) + ").expect(\"parse the expected shape\");\n")
	b.WriteString("    assert_eq!(serde_json::from_str::<serde_json::Value>(&text).expect(\"reparse\"), expected);\n")
	b.WriteString("    let back: " + ty + " = serde_json::from_str(&text).expect(\"round trip\");\n")
	b.WriteString("    assert_eq!(back, value);\n")
	b.WriteString("}\n")
	return b.String(), nil
}

// expectedShape binds the JSON that serialising the sample must produce. When
// it is the fixture verbatim the fixture is reused, so that the cases where the
// two genuinely differ — a null container normalising to an empty one, an
// absent optional staying absent — are visible in the emitted test rather than
// buried in two near-identical literals.
func expectedShape(
	v sample,
) string {
	if v.jsonIn == v.jsonOut {
		return "    let expected: serde_json::Value =\n" +
			"        serde_json::from_str(wire).expect(\"parse the fixture\");\n"
	}
	return "    let expected: serde_json::Value =\n" +
		"        serde_json::from_str(" + rustRawStr(v.jsonOut) + ")\n" +
		"            .expect(\"parse the expected shape\");\n"
}

// declaresValue reports whether any constant of the enum carries the value.
func declaresValue(
	d Decl,
	value string,
) bool {
	for _, v := range d.Variants {
		if v.Value == value {
			return true
		}
	}
	return false
}

// variantNames replays rustEnum's naming so the tests name the same variants
// the DTOs declare. Sharing the function rather than the rule is the point: a
// change to variantName cannot leave the tests naming something that no longer
// exists, because both call it.
func variantNames(
	d Decl,
) []string {
	used := map[string]bool{}
	out := make([]string, 0, len(d.Variants))
	for _, v := range d.Variants {
		out = append(out, variantName(d, v, used))
	}
	return out
}

// fresh runs one sample walk with its own counter state and an empty cycle
// stack, so each emitted test reads with values numbered from 1.
func (s *sampler) fresh(
	build func() (sample, error),
) (sample, error) {
	s.n = 0
	s.stack = map[string]bool{}
	s.depth = 0
	return build()
}

// structValue builds a struct literal and its two JSON forms.
func (s *sampler) structValue(
	d Decl,
	empty bool,
) (sample, error) {
	key := d.Module + "." + d.Name
	s.stack[key] = true
	defer delete(s.stack, key)
	s.depth++
	defer func() { s.depth-- }()
	if s.depth > maxSampleDepth {
		return sample{}, fmt.Errorf("sample walk exceeded %d levels at %s", maxSampleDepth, d.GoPath)
	}

	name := rustPath(d)
	var (
		fields []string
		in     []string
		out    []string
	)
	for _, f := range d.Fields {
		v, err := s.fieldValue(f, empty)
		if err != nil {
			return sample{}, err
		}
		fields = append(fields, "        "+escapeRustIdent(toSnake(f.JSONName))+": "+v.rust+",")
		if v.jsonIn != omitted {
			in = append(in, jsonStr(f.JSONName)+": "+v.jsonIn)
		}
		if v.jsonOut != omitted {
			out = append(out, jsonStr(f.JSONName)+": "+v.jsonOut)
		}
	}
	rust := name + " {}"
	if len(fields) > 0 {
		rust = name + " {\n" + strings.Join(fields, "\n") + "\n    }"
	}
	return sample{
		rust:    rust,
		jsonIn:  "{" + strings.Join(in, ", ") + "}",
		jsonOut: "{" + strings.Join(out, ", ") + "}",
	}, nil
}

// fieldValue builds one struct field, applying the serde attributes the Rust
// emitter put on it: an optional field is absent from the zero-value wire, and
// a non-optional container arrives as null there.
func (s *sampler) fieldValue(
	f Field,
	empty bool,
) (sample, error) {
	switch {
	case empty && f.Optional:
		return sample{rust: "None", jsonIn: omitted, jsonOut: omitted}, nil
	case empty && refNullable(f.Type, s.aliases):
		v, err := s.value(f.Type, true)
		if err != nil {
			return sample{}, err
		}
		v.jsonIn = "null"
		return v, nil
	}
	v, err := s.value(f.Type, empty)
	if err != nil {
		return sample{}, err
	}
	// A pointer field already lowered to an option container; the Rust emitter
	// does not wrap it twice and neither may the sample.
	if f.Optional && f.Type.Container != "option" {
		v.rust = "Some(" + v.rust + ")"
	}
	return v, nil
}

// value builds a sample for one type reference.
func (s *sampler) value(
	ref TypeRef,
	empty bool,
) (sample, error) {
	switch ref.Container {
	case "option":
		if empty {
			return sample{rust: "None", jsonIn: "null", jsonOut: "null"}, nil
		}
		inner, err := s.value(*ref.Elem, false)
		if err != nil {
			return sample{}, err
		}
		return sample{
			rust:    "Some(" + inner.rust + ")",
			jsonIn:  inner.jsonIn,
			jsonOut: inner.jsonOut,
		}, nil
	case "slice":
		if empty {
			return sample{rust: "Vec::new()", jsonIn: "[]", jsonOut: "[]"}, nil
		}
		inner, err := s.value(*ref.Elem, false)
		if err != nil {
			return sample{}, err
		}
		return sample{
			rust:    "vec![" + inner.rust + "]",
			jsonIn:  "[" + inner.jsonIn + "]",
			jsonOut: "[" + inner.jsonOut + "]",
		}, nil
	case "map":
		if empty {
			return sample{
				rust:    "std::collections::HashMap::new()",
				jsonIn:  "{}",
				jsonOut: "{}",
			}, nil
		}
		k, err := s.value(*ref.Key, false)
		if err != nil {
			return sample{}, err
		}
		v, err := s.value(*ref.Elem, false)
		if err != nil {
			return sample{}, err
		}
		rust := "{\n" +
			"        let mut m = std::collections::HashMap::new();\n" +
			"        m.insert(" + k.rust + ", " + v.rust + ");\n" +
			"        m\n" +
			"    }"
		return sample{
			rust:    rust,
			jsonIn:  "{" + asJSONKey(k.jsonIn) + ": " + v.jsonIn + "}",
			jsonOut: "{" + asJSONKey(k.jsonOut) + ": " + v.jsonOut + "}",
		}, nil
	}

	if ref.Name != "" {
		return s.namedValue(ref, empty)
	}
	return s.primValue(ref.Prim, empty)
}

// namedValue builds a sample for a reference to another declaration.
func (s *sampler) namedValue(
	ref TypeRef,
	empty bool,
) (sample, error) {
	key := ref.Module + "." + ref.Name
	d, ok := s.byKey[key]
	if !ok {
		return sample{}, fmt.Errorf("reference to %s, which was not emitted", key)
	}
	switch d.Kind {
	case KindAlias:
		return s.value(*d.Underlying, empty)
	case KindEnum:
		return s.enumValue(d, empty), nil
	case KindStruct:
		// Cycle breaker: a self-referential type reached through a container
		// gets its zero-value form, whose containers are empty, so the walk
		// terminates one level down instead of recursing forever.
		return s.structValue(d, empty || s.stack[key])
	}
	return sample{}, fmt.Errorf("declaration %s has kind %q, which has no sample form", key, d.Kind)
}

// enumValue picks a sample member of a generated enum.
func (s *sampler) enumValue(
	d Decl,
	empty bool,
) sample {
	ty := rustPath(d)
	if empty || len(d.Variants) == 0 {
		return sample{
			rust:    ty + "::Other(String::new())",
			jsonIn:  `""`,
			jsonOut: `""`,
		}
	}
	names := variantNames(d)
	v := d.Variants[0]
	return sample{
		rust:    ty + "::" + names[0],
		jsonIn:  jsonStr(v.Value),
		jsonOut: jsonStr(v.Value),
	}
}

// primValue builds a sample for a primitive. Every scalar in one sample gets a
// distinct value, so a pair of fields transposed by a bad rename cannot both
// still compare equal.
func (s *sampler) primValue(
	prim string,
	empty bool,
) (sample, error) {
	switch prim {
	case primString, primBytes:
		if empty {
			return sample{rust: "String::new()", jsonIn: `""`, jsonOut: `""`}, nil
		}
		v := fmt.Sprintf("s%d", s.next())
		return sample{rust: "String::from(" + rustStr(v) + ")", jsonIn: jsonStr(v), jsonOut: jsonStr(v)}, nil
	case primTime:
		if empty {
			return sample{rust: "String::new()", jsonIn: `""`, jsonOut: `""`}, nil
		}
		// A time.Time is a String here on purpose (§9.2): the daemon's own
		// rendering round-trips byte for byte, nanoseconds and offset included,
		// which is what a chrono type would have had to prove instead.
		v := fmt.Sprintf("2026-07-31T05:00:00.%09dZ", s.next())
		return sample{rust: "String::from(" + rustStr(v) + ")", jsonIn: jsonStr(v), jsonOut: jsonStr(v)}, nil
	case "T":
		if empty {
			return sample{rust: "String::new()", jsonIn: `""`, jsonOut: `""`}, nil
		}
		v := fmt.Sprintf("t%d", s.next())
		return sample{rust: "String::from(" + rustStr(v) + ")", jsonIn: jsonStr(v), jsonOut: jsonStr(v)}, nil
	case primBool:
		if empty {
			return sample{rust: "false", jsonIn: "false", jsonOut: "false"}, nil
		}
		// Alternating rather than always true, so a field pair that is
		// transposed shows up here too.
		if s.next()%2 == 1 {
			return sample{rust: "true", jsonIn: "true", jsonOut: "true"}, nil
		}
		return sample{rust: "false", jsonIn: "false", jsonOut: "false"}, nil
	case primI8, primI16, primI32, primI64, primU8, primU16, primU32, primU64:
		if empty {
			return sample{rust: "0", jsonIn: "0", jsonOut: "0"}, nil
		}
		v := fmt.Sprintf("%d", s.next())
		return sample{rust: v, jsonIn: v, jsonOut: v}, nil
	case primF32, primF64:
		if empty {
			// `0.0` on both sides, not `0`: serde_json keeps the distinction
			// between an integer and a float literal in its Value, so a `0`
			// here would not compare equal to the `0.0` a float field
			// serialises to.
			return sample{rust: "0.0", jsonIn: "0.0", jsonOut: "0.0"}, nil
		}
		// A half is exactly representable, so the comparison after a round trip
		// through decimal is an equality rather than an epsilon.
		v := fmt.Sprintf("%d.5", s.next())
		return sample{rust: v, jsonIn: v, jsonOut: v}, nil
	case primJSON:
		return sample{}, fmt.Errorf(
			"lowers to serde_json::Value, and §4.2 gives this crate serde and nothing else")
	}
	return sample{}, fmt.Errorf("primitive %q has no sample form", prim)
}

// asJSONKey renders a scalar's JSON form as an object key, which JSON requires
// to be a string even when the Go map key is not.
func asJSONKey(
	v string,
) string {
	if strings.HasPrefix(v, `"`) {
		return v
	}
	return `"` + v + `"`
}

// jsonStr renders a Go string as a JSON string literal.
func jsonStr(
	s string,
) string {
	return `"` + escapeForQuotes(s) + `"`
}

// rustStr renders a Go string as a Rust string literal. Written rather than
// reached for via strconv.Quote because Go escapes a non-ASCII rune as
// `\uXXXX`, which Rust does not accept — Rust spells it `\u{XXXX}`. Passing
// UTF-8 through verbatim is correct in both languages and avoids the question.
func rustStr(
	s string,
) string {
	return `"` + escapeForQuotes(s) + `"`
}

// escapeForQuotes escapes the characters that cannot appear raw inside a
// double-quoted literal in either language.
func escapeForQuotes(
	s string,
) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rustRawStr renders a JSON document as a Rust literal, preferring a raw string
// so the fixture reads as the JSON it is. It steps the hash count up until the
// delimiter cannot appear in the body, and falls back to an escaped literal if
// the body contains something a raw string cannot hold at all.
func rustRawStr(
	body string,
) string {
	if strings.ContainsRune(body, '\r') {
		return `"` + escapeForQuotes(body) + `"`
	}
	for hashes := 1; hashes <= 8; hashes++ {
		h := strings.Repeat("#", hashes)
		if !strings.Contains(body, `"`+h) {
			return "r" + h + `"` + body + `"` + h
		}
	}
	return `"` + escapeForQuotes(body) + `"`
}

// SkippedRustTestSummary renders the declarations EmitRustTests refused, for
// the coverage summary on stderr.
func SkippedRustTestSummary(
	skipped []Unresolved,
) string {
	if len(skipped) == 0 {
		return ""
	}
	names := make([]string, 0, len(skipped))
	for _, u := range skipped {
		names = append(names, "    "+u.What+": "+u.Reason)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "  round-trip tests      %d declaration(s) not covered\n", len(skipped))
	b.WriteString(strings.Join(names, "\n") + "\n")
	return b.String()
}
