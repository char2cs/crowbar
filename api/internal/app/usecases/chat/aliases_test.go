package chat_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ─── from layering_test.go ────────────────────────────────────────────

// featurePath is this feature's import path. Every edge the guard below judges is
// an edge that starts and ends inside it.
const featurePath = "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

// sharedTier is the one place a package may import outside its own subtree.
//
// Everything under it is VOCABULARY: a type or a piece of process-local state that
// two or more components name. Nothing else may live there, and its members do not
// know each other either — see rule 2 below.
const sharedTier = "internal/shared"

// components are the four halves of the chat surface: the record, the hook
// ingress, the CLI lifecycle and the provider table.
//
// They are peers, and the whole point of the decomposition is that they do not know
// about each other. They call each other constantly — a purge retires CLIs, a hook
// moves a runner, a switch waits on a turn — and every one of those calls goes
// through an interface the CALLER declares, wired by New. An import between two of
// them would put the cycle back and take the orchestrator's reason to exist with it.
var components = []string{"conversation", "turn", "runner", "provider"}

// TestLayering_TheOnlyThingThatCrossesIsSharedVocabulary is the rule that makes the
// decomposition real rather than cosmetic. In one sentence:
//
//	A package may import only its own descendants and internal/shared/*; a member of
//	internal/shared may import only its own descendants; nothing under internal/
//	imports the face.
//
// Each clause pays for itself. The first stops two components knowing each other,
// which is what dissolved the cycle between them. The second keeps the shared tier
// a flat vocabulary instead of a second architecture growing inside the first — a
// shared package that reached sideways would be behaviour wearing a smaller name.
// The third is the face's job description: it knows the components, and a component
// that knew the face would be a cycle with an extra step.
//
// Inverted and OBSERVED to fail: adding `_ ".../internal/turn"` to
// internal/runner/runner.go fails with "internal/runner must not import
// internal/turn", and adding `_ ".../internal/shared/inflight"` to
// internal/shared/telemetry/telemetry.go fails with "internal/shared/telemetry must
// not import internal/shared/inflight".
//
// The third clause could not be observed the same way, and cannot be: every package
// under internal/ is reachable from the face, so an import back to it is an import
// cycle the compiler refuses before this test runs. It is stated anyway — a rule
// that is currently unreachable is still the rule, and the day a package appears
// that the face does not import, this is what stops it reaching back.
func TestLayering_TheOnlyThingThatCrossesIsSharedVocabulary(t *testing.T) {
	t.Parallel()

	edges := featureImports(t)
	require.NotEmpty(t, edges, "no intra-feature imports were found at all; the walk is broken")

	for _, edge := range edges {
		if allowedEdge(edge.from, edge.to) {
			continue
		}
		t.Errorf("%s must not import %s\n\n%s", edge.from, edge.to, layeringRule(edge.from, edge.to))
	}
}

// TestLayering_EveryComponentIsReachedFromTheFace stops the guard above from
// passing vacuously. A rule about edges proves nothing if the edges have gone.
func TestLayering_EveryComponentIsReachedFromTheFace(t *testing.T) {
	t.Parallel()

	reached := map[string]bool{}
	for _, edge := range featureImports(t) {
		if edge.from == "" {
			reached[edge.to] = true
		}
	}
	for _, component := range components {
		require.Truef(t, reached["internal/"+component],
			"the face does not import internal/%s; either it is dead or something else is orchestrating it",
			component)
	}
}

type importEdge struct{ from, to string }

// featureImports parses every package in the feature and returns the edges that
// stay inside it, both sides expressed relative to the feature root ("" for the
// face itself).
func featureImports(t *testing.T) []importEdge {
	t.Helper()

	root, err := filepath.Abs(".")
	require.NoError(t, err)

	seen := map[importEdge]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		// Test files are excluded on purpose: a test may reach for whatever it needs
		// to build a fixture, and constraining that would only push fixtures into
		// production code.
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		from := relativePackage(root, filepath.Dir(p))
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imported != featurePath && !strings.HasPrefix(imported, featurePath+"/") {
				continue
			}
			seen[importEdge{from: from, to: strings.TrimPrefix(strings.TrimPrefix(imported, featurePath), "/")}] = true
		}
		return nil
	})
	require.NoError(t, err)

	edges := slices.Collect(maps.Keys(seen))
	slices.SortFunc(edges, func(a, b importEdge) int {
		if a.from != b.from {
			return strings.Compare(a.from, b.from)
		}
		return strings.Compare(a.to, b.to)
	})
	return edges
}

func relativePackage(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func allowedEdge(from, to string) bool {
	switch {
	case from == "":
		// The face is the orchestrator: knowing every component is its job.
		return true
	case to == "":
		// Nothing below may reach back up.
		return false
	case isDescendant(to, from):
		// A package owns its own subtree.
		return true
	case isShared(from):
		// A member of the shared tier has already had its descendants allowed above;
		// anything else it reaches for is sideways.
		return false
	default:
		return isShared(to)
	}
}

// isDescendant reports whether child is strictly inside parent's subtree.
func isDescendant(child, parent string) bool {
	return parent != "" && strings.HasPrefix(child, parent+"/")
}

// isShared reports whether pkg is a member of the shared tier — one of its
// top-level packages, not the tier directory itself and not something nested
// deeper, which belongs to the member above it.
func isShared(pkg string) bool {
	return path.Dir(pkg) == sharedTier
}

func layeringRule(from, to string) string {
	switch {
	case to == "":
		return "nothing under internal/ may import the face: the face knows the " +
			"components, and a component that knew the face would be a cycle with an " +
			"extra step."
	case isShared(from):
		return "a member of " + sharedTier + " may import nothing inside this feature " +
			"but its own descendants. The shared tier is vocabulary; one member " +
			"reaching for another is a second architecture growing inside the first."
	case isShared(to) || path.Dir(to) != "internal":
		return "a package may reach outside its own subtree only into " + sharedTier +
			". If " + from + " genuinely needs " + to + ", either the thing it needs is " +
			"vocabulary and belongs in " + sharedTier + ", or it belongs underneath " +
			from + "."
	default:
		return "the components are peers and must not know each other. Declare the " +
			"narrow interface you need in " + from + "/types.go and let New wire it — " +
			"that is what dissolves the cycle between them."
	}
}

// maxProductionFileLines is a hard ceiling, not a guideline. A production file
// past it is a decomposition that has not happened yet.
//
// It counts PRODUCTION files only. A test file may be as long as its fixtures
// need — capping them would push fixtures into production code, which is the
// outcome this rule exists to prevent.
const maxProductionFileLines = 500

// TestLayering_NoProductionFileIsOversized keeps the size rule from rotting.
//
// Six files broke it when the rule was written and each was decomposed; without a
// gate the seventh arrives silently, because a file grows one accepted diff at a
// time and no single diff looks like the problem.
func TestLayering_NoProductionFileIsOversized(t *testing.T) {
	root, err := filepath.Abs(".")
	require.NoError(t, err)

	type oversized struct {
		path  string
		lines int
	}
	var over []oversized

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Count as wc -l does: one per newline, plus one more only when the file
		// ends without a trailing newline. Counting a phantom last line would put
		// the real ceiling at 499 and reject a legal file.
		n := bytes.Count(body, []byte("\n"))
		if len(body) > 0 && body[len(body)-1] != '\n' {
			n++
		}
		if n > maxProductionFileLines {
			rel, relErr := filepath.Rel(root, p)
			if relErr != nil {
				rel = p
			}
			over = append(over, oversized{path: rel, lines: n})
		}
		return nil
	})
	require.NoError(t, err)

	// Proven by inversion: raising a file past the ceiling fails here by name.
	for _, o := range over {
		t.Errorf(
			"%s is %d lines, over the %d-line ceiling. Split it by responsibility — "+
				"not by technical kind. A file named for what it DOES can be read on "+
				"its own; a file named for the shape of its contents cannot.",
			o.path, o.lines, maxProductionFileLines,
		)
	}
}
