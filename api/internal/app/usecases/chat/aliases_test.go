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

const featurePath = "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

const sharedTier = "internal/shared"

var components = []string{"conversation", "ingress", "runner", "provider"}

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
		return true
	case to == "":
		return false
	case isDescendant(to, from):
		return true
	case isShared(from):
		return false
	default:
		return isShared(to)
	}
}

func isDescendant(child, parent string) bool {
	return parent != "" && strings.HasPrefix(child, parent+"/")
}

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

const maxProductionFileLines = 500

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

	for _, o := range over {
		t.Errorf(
			"%s is %d lines, over the %d-line ceiling. Split it by responsibility — "+
				"not by technical kind. A file named for what it DOES can be read on "+
				"its own; a file named for the shape of its contents cannot.",
			o.path, o.lines, maxProductionFileLines,
		)
	}
}
