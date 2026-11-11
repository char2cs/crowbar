package mapping_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
)

func mustPattern(t *testing.T, from, pattern string) mapping.Branch {
	t.Helper()
	branch, err := mapping.Pattern(from, pattern)
	if err != nil {
		t.Fatal(err)
	}
	return branch
}

const mcpName = `^mcp__.+?__(?P<v>.+)$`

func TestPattern_KeepsGroupVAndFallsThroughWhenItDoesNotMatch(t *testing.T) {
	paths := mapping.Expr{mustPattern(t, "name", mcpName), mapping.Path("other")}

	if got := mapping.String(map[string]any{"name": "mcp__echo__shout", "other": "kept"}, paths); got != "shout" {
		t.Fatalf("got %q, want the group", got)
	}
	if got := mapping.String(map[string]any{"name": "Bash", "other": "kept"}, paths); got != "kept" {
		t.Fatalf("got %q, want the next branch: a pattern that does not match is absent", got)
	}
	if mapping.Present(map[string]any{"name": 7}, mapping.Expr{mustPattern(t, "name", mcpName)}) {
		t.Fatal("a value that is no string must be absent")
	}
}

func TestPattern_RejectsABadRegexpAndAMissingGroupV(t *testing.T) {
	for name, pattern := range map[string]string{"syntax": `^mcp__(?P<v>.+`, "no group v": `^mcp__(.+)$`} {
		if _, err := mapping.Pattern("name", pattern); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPattern_ReadsWithoutCompiling(t *testing.T) {
	paths := mapping.Expr{mustPattern(t, "name", mcpName)}
	doc := map[string]any{"name": "Bash"}

	allocs := testing.AllocsPerRun(100, func() { mapping.String(doc, paths) })

	// Compiling per read costs ~57 allocations; the bound leaves room for pool misses under -race.
	if allocs > 20 {
		t.Fatalf("a pattern read made %.0f allocations; the regexp must be compiled once, at build", allocs)
	}
}

func TestKindMap_JoinKey(t *testing.T) {
	kinds := map[string]string{
		"commandExecution":               "execute",
		"collabAgentToolCall":            "other",
		"collabAgentToolCall/spawnAgent": "subagent",
		"collabAgentToolCall/wait":       "meta",
	}
	key := func(item map[string]any) string {
		return mapping.String(map[string]any{"item": item}, mapping.Expr{mapping.Join("item.type", "item.tool")})
	}
	if got := key(map[string]any{"type": "collabAgentToolCall", "tool": "wait"}); got != "collabAgentToolCall/wait" {
		t.Fatalf("join = %q, want the values joined with /", got)
	}
	if got := key(map[string]any{"type": "commandExecution"}); got != "commandExecution" {
		t.Fatalf("join = %q, want a missing part left out", got)
	}
	if got := key(map[string]any{"tool": "wait"}); got != "wait" {
		t.Fatalf("join = %q, want a missing leading part skipped, leaving the bare remainder", got)
	}
	if mapping.Present(map[string]any{}, mapping.Expr{mapping.Join("a", "b")}) {
		t.Fatal("a join of nothing present must be absent")
	}
	for key, want := range map[string]string{
		"collabAgentToolCall/spawnAgent": "subagent",
		"collabAgentToolCall/wait":       "meta",
		"collabAgentToolCall/closeAgent": "other",
		"commandExecution":               "execute",
	} {
		if got, ok := mapping.LookupKind(kinds, key); !ok || got != want {
			t.Errorf("LookupKind(%q) = (%q,%v), want %q", key, got, ok, want)
		}
	}
	if got, ok := mapping.LookupKind(kinds, "webSearch/x"); ok {
		t.Fatalf("LookupKind of an unmapped key = %q, want none", got)
	}
}

func TestExpr_LiteralIsTheFirstBranchAsWritten(t *testing.T) {
	if got := mapping.Paths("Allow once", "x").Literal(); got != "Allow once" {
		t.Fatalf("Literal = %q", got)
	}
	if got := mapping.Expr(nil).Literal(); got != "" {
		t.Fatalf("Literal of nothing = %q", got)
	}
}
