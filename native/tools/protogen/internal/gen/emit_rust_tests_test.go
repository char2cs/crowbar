package gen

import (
	"regexp"
	"strings"
	"testing"
)

// rustTestsFor generates the round-trip suite from the fixture and returns the
// single emitted file plus what it refused.
func rustTestsFor(
	t *testing.T,
) (string, []Unresolved) {
	t.Helper()
	files, skipped := EmitRustTests(fixtureRun(t))
	body, ok := files[RustTestsFile]
	if !ok {
		t.Fatalf("no %s emitted; got %v", RustTestsFile, files)
	}
	return body, skipped
}

// testBody returns the emitted source of one test function.
func testBody(
	t *testing.T,
	source string,
	name string,
) string {
	t.Helper()
	marker := "\nfn " + name + "() {\n"
	start := strings.Index(source, marker)
	if start < 0 {
		t.Fatalf("test %s was not emitted; emitted %v", name, emittedTestNames(source))
	}
	rest := source[start+len(marker):]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("test %s has no closing brace", name)
	}
	return rest[:end]
}

// testFnRe matches an emitted test function's name.
var testFnRe = regexp.MustCompile(`(?m)^fn ([A-Za-z0-9_]+)\(\) \{$`)

// emittedTestNames lists every emitted test function name.
func emittedTestNames(
	source string,
) []string {
	out := []string{}
	for _, m := range testFnRe.FindAllStringSubmatch(source, -1) {
		out = append(out, m[1])
	}
	return out
}

// TestRustTestsCoverEveryCarryableDecl asserts the suite is complete: every
// declaration the crate can actually carry has a test, and the only ones
// without are the ones whose values cannot be written under the §4.2
// dependency contract. Completeness is the property that matters — a generated
// suite that quietly stopped covering a type would look exactly like a passing
// one.
func TestRustTestsCoverEveryCarryableDecl(t *testing.T) {
	r := fixtureRun(t)
	source, skipped := rustTestsFor(t)

	skippedByPath := map[string]bool{}
	for _, u := range skipped {
		skippedByPath[u.What] = true
	}

	names := map[string]bool{}
	for _, n := range emittedTestNames(source) {
		names[n] = true
	}

	for _, d := range r.Decls {
		want := []string{}
		switch d.Kind {
		case KindStruct:
			want = []string{testName(d, "wire_shape"), testName(d, "zero_values")}
		case KindEnum:
			want = []string{testName(d, "variants")}
		case KindAlias:
			want = []string{testName(d, "alias_round_trip")}
		}
		if skippedByPath[d.GoPath] {
			for _, n := range want {
				if names[n] {
					t.Errorf("%s was reported skipped but %s was still emitted", d.GoPath, n)
				}
			}
			continue
		}
		for _, n := range want {
			if !names[n] {
				t.Errorf("%s (%s) has no emitted test %s", d.GoPath, d.Kind, n)
			}
		}
	}
}

// TestRustTestsSkipOnlyUncarryableTypes asserts the refusal is narrow: the
// fixture's Item carries an untyped map and a json.RawMessage, both of which
// lower to serde_json::Value, and it is the only struct that may be skipped.
// A generator that skipped more than it had to would hide real types behind a
// contract it was not actually hitting.
func TestRustTestsSkipOnlyUncarryableTypes(t *testing.T) {
	_, skipped := rustTestsFor(t)

	got := map[string]string{}
	for _, u := range skipped {
		got[u.What] = u.Reason
		if u.Category != "untestable-type" {
			t.Errorf("%s: category %q, want untestable-type", u.What, u.Category)
		}
		if !strings.Contains(u.Reason, "serde_json::Value") {
			t.Errorf("%s: reason %q does not name the dependency that forced it", u.What, u.Reason)
		}
	}

	// The fixture's gin stub reports itself under gin's real import path, which
	// is what makes it a stub rather than a different package.
	want := []string{
		"example.com/fixture/types.Item",
		"github.com/gin-gonic/gin.H",
	}
	for _, path := range want {
		if _, ok := got[path]; !ok {
			t.Errorf("%s reaches serde_json::Value but was not reported skipped; skipped %v", path, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("skipped %d declarations, want %d: %v", len(got), len(want), got)
	}
}

// TestRustTestsPinTheWireNames asserts the emitted fixtures are written in the
// daemon's JSON keys rather than in Rust field names. This is the property a
// round trip alone cannot prove: a struct whose rename is wrong still
// round-trips through itself perfectly, and only a fixture written in the
// daemon's own spelling catches it.
func TestRustTestsPinTheWireNames(t *testing.T) {
	// Synthetic, for the same reason as the scalar test: every carryable
	// fixture field has a JSON name that is already identical to its Rust one,
	// so the assertion would hold just as well against a generator that wrote
	// Rust names into the fixture. These three do not.
	r := &Result{Decls: []Decl{{
		Module: "synthetic",
		Name:   "Renamed",
		GoPath: "example.com/synthetic.Renamed",
		Kind:   KindStruct,
		Fields: []Field{
			{JSONName: "createdAt", GoName: "CreatedAt", Type: TypeRef{Prim: primTime}},
			{JSONName: "prUrl", GoName: "PRURL", Type: TypeRef{Prim: primString}},
			{JSONName: "type", GoName: "Type", Type: TypeRef{Prim: primString}},
		},
	}}}
	files, _ := EmitRustTests(r)
	body := testBody(t, files[RustTestsFile], "synthetic_renamed_wire_shape")

	for _, wire := range []string{`"createdAt":`, `"prUrl":`, `"type":`} {
		if !strings.Contains(body, wire) {
			t.Errorf("the fixture does not carry the wire key %s:\n%s", wire, body)
		}
	}
	for _, rust := range []string{`"created_at":`, `"pr_url":`, `"type_":`} {
		if strings.Contains(body, rust) {
			t.Errorf("the fixture carries the Rust field name %s instead of the wire key:\n%s", rust, body)
		}
	}
	// ... while the Rust literal must use the Rust names, keyword escape and all.
	for _, field := range []string{"created_at:", "pr_url:", "type_:"} {
		if !strings.Contains(body, field) {
			t.Errorf("the Rust literal does not name the field %s:\n%s", field, body)
		}
	}

	// The same property for an enum: the table is keyed by the wire value, not
	// by the Rust variant spelling derived from it.
	fixture := fixtureRun(t)
	status := findDecl(t, fixture, "Status")
	variants := testBody(t, mustSource(t), testName(status, "variants"))
	if !strings.Contains(variants, `"pr-conflicts"`) {
		t.Errorf("the variant table does not carry the hyphenated wire value:\n%s", variants)
	}
	if !strings.Contains(variants, "Status::PrConflicts") {
		t.Errorf("the variant table does not name the Rust variant:\n%s", variants)
	}
}

// TestRustTestsExerciseNullCoercion asserts the zero-value fixture feeds `null`
// for a non-optional container and expects the empty collection back out. That
// pair is the only thing in the whole suite that reaches the generated
// null_to_default helper, so if it stopped being emitted the helper would go
// untested while every test still passed.
func TestRustTestsExerciseNullCoercion(t *testing.T) {
	r := fixtureRun(t)
	d := findDecl(t, r, "TreeNode")
	body := testBody(t, mustSource(t), testName(d, "zero_values"))

	if !strings.Contains(body, `"children": null`) {
		t.Errorf("the zero-value fixture does not feed null for the nil slice:\n%s", body)
	}
	if !strings.Contains(body, `"children": []`) {
		t.Errorf("the zero-value fixture does not expect the normalised empty slice back:\n%s", body)
	}
}

// TestRustTestsTerminateOnSelfReference asserts the sample walk breaks the
// cycle a self-referential type creates. TreeNode holds a slice of itself; a
// walk that recursed on it would not return at all, so this test failing looks
// like a hang rather than a diff.
func TestRustTestsTerminateOnSelfReference(t *testing.T) {
	r := fixtureRun(t)
	d := findDecl(t, r, "TreeNode")
	body := testBody(t, mustSource(t), testName(d, "wire_shape"))

	// One level of nesting, then the inner node's own slice is empty.
	if strings.Count(body, "TreeNode {") < 2 {
		t.Errorf("the populated fixture does not nest the self-reference at all:\n%s", body)
	}
	if !strings.Contains(body, "children: Vec::new()") {
		t.Errorf("the nested self-reference does not bottom out:\n%s", body)
	}
}

// scalarRe matches the generated string scalars inside one emitted test. It
// matches any body, not a shape: a time.Time lowers to a String too and its
// sample looks like a timestamp, and it has to be distinct for the same reason
// a plain string does.
var scalarRe = regexp.MustCompile(`String::from\("([^"]*)"\)`)

// wideStruct is an IR built by hand rather than loaded from the fixture,
// because the fixture cannot express what this test needs: its only carryable
// structs have at most one string field each, so an assertion about *distinct*
// scalars run over them can never fail. That was not a hypothetical — the first
// version of this test was written against the fixture, and a mutation that
// made `next()` return a constant left it green. A guard that cannot fail is
// worse than no guard, so the input is constructed here instead.
func wideStruct() *Result {
	field := func(name string) Field {
		return Field{JSONName: name, GoName: strings.ToUpper(name), Type: TypeRef{Prim: primString}}
	}
	r := &Result{Decls: []Decl{{
		Module: "synthetic",
		Name:   "Wide",
		GoPath: "example.com/synthetic.Wide",
		Kind:   KindStruct,
		Fields: []Field{
			field("first"),
			field("second"),
			field("third"),
			{JSONName: "when", GoName: "When", Type: TypeRef{Prim: primTime}},
			{JSONName: "also", GoName: "Also", Type: TypeRef{Prim: primTime}},
			{JSONName: "count", GoName: "Count", Type: TypeRef{Prim: primI64}},
			{JSONName: "other", GoName: "Other", Type: TypeRef{Prim: primI64}},
		},
	}}}
	return r
}

// TestRustTestsUseDistinctScalars asserts no two same-typed fields in one
// sample share a value. Without it, a generator bug that transposed two fields
// would produce a suite that still passed: the assertion would compare two
// equal values and see nothing wrong.
func TestRustTestsUseDistinctScalars(t *testing.T) {
	files, skipped := EmitRustTests(wideStruct())
	if len(skipped) != 0 {
		t.Fatalf("nothing here needs a dependency, but %v was skipped", skipped)
	}
	body := testBody(t, files[RustTestsFile], "synthetic_wide_wire_shape")

	seen := map[string]bool{}
	for _, m := range scalarRe.FindAllStringSubmatch(body, -1) {
		if seen[m[1]] {
			t.Errorf("the scalar %q is reused, so a transposed pair of string fields would still compare equal:\n%s",
				m[1], body)
		}
		seen[m[1]] = true
	}
	if len(seen) < 5 {
		t.Fatalf("expected the five string-shaped fields to produce five scalars, got %d:\n%s", len(seen), body)
	}

	// The integers have to be distinct for the same reason, and they are not
	// matched by scalarRe.
	if !strings.Contains(body, "count: 6,") || !strings.Contains(body, "other: 7,") {
		t.Errorf("the integer fields are not distinct:\n%s", body)
	}
}

// TestRustTestsAreDeterministic asserts two runs emit the same bytes. The
// suite is committed output, so map iteration order leaking into it would show
// up as a spurious diff on every regeneration — the classic way a generator
// becomes noise people stop reading.
func TestRustTestsAreDeterministic(t *testing.T) {
	opts := fixtureOptions(t)
	first, err := Generate(opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := Generate(opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	a, skippedA := EmitRustTests(first)
	b, skippedB := EmitRustTests(second)
	if len(a) != len(b) {
		t.Fatalf("file count %d vs %d", len(a), len(b))
	}
	for name, body := range a {
		if b[name] != body {
			t.Errorf("%s differs between runs", name)
		}
	}
	if len(skippedA) != len(skippedB) {
		t.Errorf("skipped count %d vs %d", len(skippedA), len(skippedB))
	}

	// And a second emit from the *same* result must not drift either, which is
	// what would happen if the sampler's counter leaked across calls.
	again, _ := EmitRustTests(first)
	for name, body := range a {
		if again[name] != body {
			t.Errorf("%s differs when emitted twice from one result", name)
		}
	}
}

// mustSource memoises the emitted suite for the assertions above.
func mustSource(
	t *testing.T,
) string {
	t.Helper()
	source, _ := rustTestsFor(t)
	return source
}
