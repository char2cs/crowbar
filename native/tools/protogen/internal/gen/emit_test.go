package gen

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateGolden rewrites the golden files instead of comparing against them.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden from the fixture run")

// goldenDir is where the emitted fixture output is pinned.
func goldenDir(
	t *testing.T,
) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "golden"))
	if err != nil {
		t.Fatalf("resolve golden dir: %v", err)
	}
	return dir
}

// TestEmitGolden pins every emitted byte of the fixture surface. It is the
// backstop for the field-level tables: a change in spacing, attribute order or
// declaration order shows up here even when no table covers it.
func TestEmitGolden(t *testing.T) {
	r := fixtureRun(t)
	manifest, err := MarshalManifest(BuildManifest(r))
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	files := map[string]string{}
	for name, body := range EmitRust(r) {
		files["rust/"+name] = body
	}
	for name, body := range EmitTS(r) {
		files["ts/"+name] = body
	}
	files["manifest.json"] = string(manifest)

	dir := goldenDir(t)
	if *updateGolden {
		writeGolden(t, dir, files)
		return
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read golden: %v (run go test -update-golden)", err)
			}
			if string(want) != files[name] {
				t.Errorf("emitted output drifted from golden.\n--- want ---\n%s\n--- got ---\n%s",
					string(want), files[name])
			}
		})
	}
	assertNoStaleGolden(t, dir, files)
}

// writeGolden rewrites the golden tree from a run's output.
func writeGolden(
	t *testing.T,
	dir string,
	files map[string]string,
) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("clear golden dir: %v", err)
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
}

// assertNoStaleGolden fails when the golden tree holds a file the generator no
// longer emits, which would otherwise hide a deleted module.
func assertNoStaleGolden(
	t *testing.T,
	dir string,
	files map[string]string,
) {
	t.Helper()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if _, ok := files[filepath.ToSlash(rel)]; !ok {
			t.Errorf("stale golden file %s is no longer emitted", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk golden dir: %v", err)
	}
}

// TestGenerateIsDeterministic asserts two independent runs over the same tree
// produce byte-identical output. Map iteration order is the classic way a
// generator becomes a source of spurious diffs, so this is a load-bearing
// property, not a nicety.
func TestGenerateIsDeterministic(t *testing.T) {
	opts := fixtureOptions(t)
	first, err := Generate(opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := Generate(opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	compare := func(name string, a, b map[string]string) {
		t.Helper()
		if len(a) != len(b) {
			t.Fatalf("%s: file count %d vs %d", name, len(a), len(b))
		}
		for file, body := range a {
			if b[file] != body {
				t.Errorf("%s/%s differs between runs", name, file)
			}
		}
	}
	compare("rust", EmitRust(first), EmitRust(second))
	compare("ts", EmitTS(first), EmitTS(second))

	m1, err := MarshalManifest(BuildManifest(first))
	if err != nil {
		t.Fatalf("marshal first manifest: %v", err)
	}
	m2, err := MarshalManifest(BuildManifest(second))
	if err != nil {
		t.Fatalf("marshal second manifest: %v", err)
	}
	if string(m1) != string(m2) {
		t.Errorf("manifest differs between runs")
	}
}

// TestEmittedFilesCarryGeneratedHeader asserts every emitted file is marked, so
// the writer can prune its own output without touching hand-written files.
func TestEmittedFilesCarryGeneratedHeader(t *testing.T) {
	r := fixtureRun(t)
	for name, body := range EmitRust(r) {
		if !strings.HasPrefix(body, GeneratedHeader) {
			t.Errorf("rust/%s has no generated header", name)
		}
	}
	for name, body := range EmitTS(r) {
		if !strings.HasPrefix(body, tsGeneratedHeader) {
			t.Errorf("ts/%s has no generated header", name)
		}
	}
}

// TestEnumsCarryOpenSetFallback asserts both emitters keep a Go string type an
// OPEN set. A closed enum would reject the zero value "" that every Go string
// field can carry, turning a perfectly valid daemon response into a runtime
// deserialise error.
func TestEnumsCarryOpenSetFallback(t *testing.T) {
	r := fixtureRun(t)
	status := findDecl(t, r, "Status")

	rust := rustEnum(status)
	if !strings.Contains(rust, "#[serde(untagged)]") || !strings.Contains(rust, "Other(String)") {
		t.Errorf("rust enum has no open-set fallback:\n%s", rust)
	}
	for _, want := range []string{`rename = "new"`, `rename = "locked"`, `rename = "pr-conflicts"`} {
		if !strings.Contains(rust, want) {
			t.Errorf("rust enum missing %s:\n%s", want, rust)
		}
	}
	if !strings.Contains(rust, "PrConflicts") {
		t.Errorf("dashed wire value did not become a variant identifier:\n%s", rust)
	}

	ts := tsEnum(status)
	if !strings.Contains(ts, "(string & {})") {
		t.Errorf("ts union has no open-set fallback:\n%s", ts)
	}
	for _, want := range []string{`"new"`, `"locked"`, `"pr-conflicts"`} {
		if !strings.Contains(ts, want) {
			t.Errorf("ts union missing %s:\n%s", want, ts)
		}
	}
}

// TestNullableContainersAreCoerced asserts a non-optional slice field survives
// the JSON null a nil Go slice marshals to: Rust routes it through the
// null-coercing deserialiser, and TypeScript admits null in the type.
func TestNullableContainersAreCoerced(t *testing.T) {
	r := fixtureRun(t)
	aliases := nullableAliases(r)
	item := findDecl(t, r, "Item")

	rust := rustStruct(item, aliases)
	if !strings.Contains(rust, `deserialize_with = "super::null_default::null_to_default"`) {
		t.Errorf("nullable slice field is not null-coerced:\n%s", rust)
	}
	ts := tsStruct(item, "fixture_types", aliases)
	if !strings.Contains(ts, "tags: string[] | null;") {
		t.Errorf("nullable slice field is not typed nullable:\n%s", ts)
	}
	if !strings.Contains(ts, "description?: string;") {
		t.Errorf("omitempty field is not optional:\n%s", ts)
	}
}

// TestTSNamespacesAvoidCollisions asserts the TypeScript barrel re-exports each
// module as a namespace, which is what makes two Go packages able to declare
// the same type name.
func TestTSNamespacesAvoidCollisions(t *testing.T) {
	r := fixtureRun(t)
	index := EmitTS(r)["index.ts"]
	for _, module := range r.Modules() {
		want := "export * as " + module + " from \"./" + module + "\";"
		if !strings.Contains(index, want) {
			t.Errorf("index.ts missing %q:\n%s", want, index)
		}
	}
}
