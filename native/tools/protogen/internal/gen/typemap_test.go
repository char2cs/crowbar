package gen

import (
	"strings"
	"testing"
)

// TestItemFields is the encoding/json lowering table: every field of the
// fixture's Item, with the wire name, the optionality and the two rendered
// types it must produce. It is the single place the two emitters are held to
// the same answer.
func TestItemFields(t *testing.T) {
	r := fixtureRun(t)
	item := findDecl(t, r, "Item")
	aliases := nullableAliases(r)

	tests := []struct {
		name     string
		json     string
		optional bool
		rust     string
		ts       string
	}{
		{
			name: "plain string", json: "name", optional: false,
			rust: "String", ts: "string",
		},
		{
			name: "omitempty string is optional", json: "description", optional: true,
			rust: "String", ts: "string",
		},
		{
			name: "pointer is optional and nullable", json: "parent", optional: true,
			rust: "Option<String>", ts: "string | null",
		},
		{
			name: "pointer to struct without omitempty is still nullable", json: "child", optional: true,
			rust: "Option<super::fixture_types::Nested>", ts: "Nested | null",
		},
		{
			name: "slice without omitempty is non-optional but nullable", json: "tags", optional: false,
			rust: "Vec<String>", ts: "string[]",
		},
		{
			name: "slice with omitempty is optional", json: "items", optional: true,
			rust: "Vec<super::fixture_types::Nested>", ts: "Nested[]",
		},
		{
			name: "map of ints", json: "counts", optional: false,
			rust: "std::collections::HashMap<String, i64>", ts: "Record<string, number>",
		},
		{
			name: "map of any is arbitrary JSON", json: "extra", optional: true,
			rust: "std::collections::HashMap<String, serde_json::Value>", ts: "Record<string, unknown>",
		},
		{
			name: "json.RawMessage is arbitrary JSON", json: "raw", optional: true,
			rust: "serde_json::Value", ts: "unknown",
		},
		{
			name: "byte slice is a base64 string", json: "blob", optional: true,
			rust: "String", ts: "string",
		},
		{
			name: "named string with constants is an enum", json: "status", optional: true,
			rust: "super::fixture_types::Status", ts: "Status",
		},
		{
			name: "named string without constants is an alias", json: "kind", optional: false,
			rust: "super::fixture_types::Untagged", ts: "Untagged",
		},
		{
			name: "time.Time is an RFC 3339 string", json: "createdAt", optional: false,
			rust: "String", ts: "string",
		},
		{
			name: "int64 is a number, not a string", json: "size", optional: false,
			rust: "i64", ts: "number",
		},
		{
			name: "float", json: "ratio", optional: false,
			rust: "f64", ts: "number",
		},
		{
			name: "omitempty bool is optional", json: "done", optional: true,
			rust: "bool", ts: "boolean",
		},
		{
			name: "tagged embedded struct stays nested", json: "meta", optional: false,
			rust: "super::fixture_types::Meta", ts: "Meta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := findField(t, item, tt.json)
			if f.Optional != tt.optional {
				t.Errorf("optional = %v, want %v", f.Optional, tt.optional)
			}
			if got := rustType(f.Type); got != tt.rust {
				t.Errorf("rust = %q, want %q", got, tt.rust)
			}
			if got := tsType(f.Type, "fixture_types"); got != tt.ts {
				t.Errorf("ts = %q, want %q", got, tt.ts)
			}
			_ = aliases
		})
	}
}

// TestEmbeddedStructIsFlattened asserts an untagged embedded struct has its
// fields promoted the way encoding/json promotes them, rather than nested
// under the embedded type's name.
func TestEmbeddedStructIsFlattened(t *testing.T) {
	r := fixtureRun(t)
	item := findDecl(t, r, "Item")

	// Base is embedded with no json tag: its fields belong to Item.
	findField(t, item, "id")
	findField(t, item, "createdAt")
	for _, f := range item.Fields {
		if f.JSONName == "Base" {
			t.Fatalf("untagged embedded struct was nested instead of flattened")
		}
	}
	// The promoted fields come first, in the embedded type's own order.
	if item.Fields[0].JSONName != "id" || item.Fields[1].JSONName != "createdAt" {
		t.Fatalf("promoted field order = %v, want id then createdAt", fieldNamesOf(item))
	}
}

// TestSkippedFields asserts json:"-" and unexported fields never reach the
// wire types.
func TestSkippedFields(t *testing.T) {
	r := fixtureRun(t)
	item := findDecl(t, r, "Item")
	for _, f := range item.Fields {
		switch f.GoName {
		case "Internal":
			t.Errorf(`field tagged json:"-" was emitted`)
		case "unexported":
			t.Errorf("unexported field was emitted")
		}
		if f.JSONName == "-" {
			t.Errorf(`field emitted with JSON name "-"`)
		}
	}
}

// TestStringEnumDetection asserts a named string type with a package-level
// constant set becomes an enum, its variants sorted by wire value, and that a
// named string without constants stays an alias.
func TestStringEnumDetection(t *testing.T) {
	r := fixtureRun(t)

	status := findDecl(t, r, "Status")
	if status.Kind != KindEnum {
		t.Fatalf("Status kind = %s, want enum", status.Kind)
	}
	want := []string{"locked", "new", "pr-conflicts"}
	if len(status.Variants) != len(want) {
		t.Fatalf("variants = %v, want %v", status.Variants, want)
	}
	for i, v := range status.Variants {
		if v.Value != want[i] {
			t.Errorf("variant %d = %q, want %q", i, v.Value, want[i])
		}
	}

	untagged := findDecl(t, r, "Untagged")
	if untagged.Kind != KindAlias {
		t.Fatalf("Untagged kind = %s, want alias", untagged.Kind)
	}
	if untagged.Underlying == nil || untagged.Underlying.Prim != primString {
		t.Fatalf("Untagged underlying = %v, want string", untagged.Underlying)
	}
}

// TestSelfReferentialTypeTerminates asserts the transitive closure emits a
// self-referential type once and refers back to it, instead of recursing until
// the depth guard trips.
func TestSelfReferentialTypeTerminates(t *testing.T) {
	r := fixtureRun(t)
	node := findDecl(t, r, "TreeNode")
	children := findField(t, node, "children")
	if children.Type.Container != "slice" || children.Type.Elem == nil {
		t.Fatalf("children = %v, want a slice", children.Type)
	}
	if children.Type.Elem.Name != "TreeNode" {
		t.Fatalf("children element = %q, want TreeNode", children.Type.Elem.Name)
	}
	count := 0
	for _, d := range r.Decls {
		if d.Name == "TreeNode" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("TreeNode emitted %d times, want once", count)
	}
}

// TestDroppedFieldIsReportedNotHidden is the anti-silent-skip guard. A field
// the generator cannot lower must (a) be recorded on the declaration, (b) be
// banner-marked in both emitted languages, and (c) make every endpoint that
// reaches the type report unresolved — because the generated struct would
// otherwise compile, deserialise, and quietly never carry that key.
func TestDroppedFieldIsReportedNotHidden(t *testing.T) {
	r := fixtureRun(t)
	lossy := findDecl(t, r, "Lossy")

	if len(lossy.Dropped) != 1 {
		t.Fatalf("dropped = %v, want exactly one entry", lossy.Dropped)
	}
	if !strings.Contains(lossy.Dropped[0].What, "Rows") {
		t.Errorf("dropped entry names %q, want the Rows field", lossy.Dropped[0].What)
	}
	for _, f := range lossy.Fields {
		if f.GoName == "Rows" {
			t.Errorf("unlowerable field was emitted anyway")
		}
	}

	rust := rustStruct(lossy, nullableAliases(r))
	if !strings.Contains(rust, "INCOMPLETE") {
		t.Errorf("rust struct carries no INCOMPLETE banner:\n%s", rust)
	}
	ts := tsStruct(lossy, "fixture_types", nullableAliases(r))
	if !strings.Contains(ts, "INCOMPLETE") {
		t.Errorf("ts interface carries no INCOMPLETE banner:\n%s", ts)
	}

	e := findEndpoint(t, r, "GET", "/v0/projects/:projectId/lossy")
	if e.FullyResolved() {
		t.Fatalf("endpoint returning an incomplete DTO was counted as fully resolved")
	}
	if !hasCategory(e.Unresolved, "incomplete-type") {
		t.Errorf("unresolved = %v, want an incomplete-type entry", e.Unresolved)
	}
}

// TestToSnake covers the JSON-name to Rust-field conversion, including the
// acronym runs the daemon's camelCase names produce.
func TestToSnake(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "already snake", in: "hunk_id", want: "hunk_id"},
		{name: "camel", in: "hunkId", want: "hunk_id"},
		{name: "leading acronym", in: "prUrl", want: "pr_url"},
		{name: "trailing acronym", in: "workspaceID", want: "workspace_id"},
		{name: "interior acronym", in: "URLValue", want: "url_value"},
		{name: "single word", in: "id", want: "id"},
		{name: "digits attach", in: "sha256Sum", want: "sha256_sum"},
		{name: "dashes become underscores", in: "pr-conflicts", want: "pr_conflicts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toSnake(tt.in); got != tt.want {
				t.Errorf("toSnake(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCamelRoundTrip asserts the check that decides between one container-level
// rename_all and a rename on every field: it must only claim a round trip when
// there really is one.
func TestCamelRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		round bool
	}{
		{name: "camel round trips", json: "hunkId", round: true},
		{name: "acronym round trips", json: "prUrl", round: true},
		{name: "single word round trips", json: "id", round: true},
		{name: "snake on the wire does not", json: "file_path", round: false},
		{name: "dashes do not", json: "pr-conflicts", round: false},
		{name: "leading capital does not", json: "Author", round: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toCamel(toSnake(tt.json)) == tt.json
			if got != tt.round {
				t.Errorf("round trip for %q = %v, want %v", tt.json, got, tt.round)
			}
		})
	}
}

// TestRustKeywordFieldIsEscaped asserts a JSON name that collides with a Rust
// keyword is escaped and carries an explicit rename, and that its struct falls
// back to per-field renames because rename_all can no longer reproduce it.
func TestRustKeywordFieldIsEscaped(t *testing.T) {
	r := fixtureRun(t)
	item := findDecl(t, r, "Item")
	if allFieldsCamel(item.Fields) {
		t.Fatalf("struct with a keyword field claimed a camelCase round trip")
	}
	out := rustStruct(item, nullableAliases(r))
	if !strings.Contains(out, `#[serde(rename = "type")]`) {
		t.Errorf("no rename for the `type` field:\n%s", out)
	}
	if !strings.Contains(out, "pub type_: String,") {
		t.Errorf("`type` field not escaped:\n%s", out)
	}
}
