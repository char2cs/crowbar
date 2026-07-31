package gen

import (
	"path/filepath"
	"sync"
	"testing"
)

// fixtureOptions points protogen at testdata/fixture: a self-contained Go
// module that replaces gin with a stub, so the tests type-check offline and in
// milliseconds while still exercising the real go/packages path.
func fixtureOptions(
	t *testing.T,
) Options {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixture"))
	if err != nil {
		t.Fatalf("resolve fixture root: %v", err)
	}
	return Options{
		DaemonRoot:  root,
		Patterns:    []string{"./..."},
		EntryPkg:    "example.com/fixture",
		EntryFunc:   "Register",
		MountPrefix: "/v0",
		ResponsePkg: "example.com/fixture/libs",
	}
}

var (
	fixtureOnce   sync.Once
	fixtureResult *Result
	fixtureErr    error
)

// fixtureRun generates from the fixture module once per test binary; loading
// and type-checking is the expensive half and every test wants the same
// answer.
func fixtureRun(
	t *testing.T,
) *Result {
	t.Helper()
	fixtureOnce.Do(func() {
		fixtureResult, fixtureErr = Generate(fixtureOptions(t))
	})
	if fixtureErr != nil {
		t.Fatalf("generate fixture: %v", fixtureErr)
	}
	return fixtureResult
}

// findDecl returns the emitted declaration with the given name.
func findDecl(
	t *testing.T,
	r *Result,
	name string,
) Decl {
	t.Helper()
	for _, d := range r.Decls {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("declaration %q not emitted; got %v", name, declNamesOf(r))
	return Decl{}
}

// declNamesOf lists every emitted declaration name, for failure messages.
func declNamesOf(
	r *Result,
) []string {
	out := make([]string, 0, len(r.Decls))
	for _, d := range r.Decls {
		out = append(out, d.Module+"."+d.Name)
	}
	return out
}

// findField returns a struct's field by JSON name.
func findField(
	t *testing.T,
	d Decl,
	jsonName string,
) Field {
	t.Helper()
	for _, f := range d.Fields {
		if f.JSONName == jsonName {
			return f
		}
	}
	t.Fatalf("field %q not emitted on %s; got %v", jsonName, d.Name, fieldNamesOf(d))
	return Field{}
}

// fieldNamesOf lists a struct's emitted JSON names, for failure messages.
func fieldNamesOf(
	d Decl,
) []string {
	out := make([]string, 0, len(d.Fields))
	for _, f := range d.Fields {
		out = append(out, f.JSONName)
	}
	return out
}

// findEndpoint returns the discovered endpoint for a method and path.
func findEndpoint(
	t *testing.T,
	r *Result,
	method string,
	path string,
) Endpoint {
	t.Helper()
	for _, e := range r.Endpoints {
		if e.Method == method && e.Path == path {
			return e
		}
	}
	t.Fatalf("endpoint %s %s not discovered", method, path)
	return Endpoint{}
}
