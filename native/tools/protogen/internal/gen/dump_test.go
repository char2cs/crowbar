package gen

import (
	"fmt"
	"testing"
)

// TestDumpFixture is a development aid: run it with -v to see everything the
// fixture module produces. It asserts nothing.
func TestDumpFixture(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("dump only runs with -v")
	}
	r := fixtureRun(t)
	for _, e := range r.Endpoints {
		fmt.Printf("%-6s %-40s handler=%-12s kind=%-10s req=%v resp=%v unresolved=%d\n",
			e.Method, e.Path, e.Handler, e.ResponseKind, e.Request, e.Responses, len(e.Unresolved))
	}
	for _, d := range r.Decls {
		fmt.Printf("DECL %s.%s kind=%s fields=%d variants=%d\n", d.Module, d.Name, d.Kind, len(d.Fields), len(d.Variants))
		for _, f := range d.Fields {
			fmt.Printf("    %-14s optional=%-5v %s\n", f.JSONName, f.Optional, rustType(f.Type))
		}
	}
	for _, u := range r.Unresolved {
		fmt.Printf("UNRESOLVED [%s/%s] %s: %s\n", u.Severity, u.Category, u.What, u.Reason)
	}
	for name, body := range EmitRust(r) {
		fmt.Printf("=== rust %s ===\n%s\n", name, body)
	}
	for name, body := range EmitTS(r) {
		fmt.Printf("=== ts %s ===\n%s\n", name, body)
	}
}
