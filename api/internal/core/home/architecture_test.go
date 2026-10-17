package home

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowedFiles may name the Crowbar home themselves, keyed by slash path
// relative to api/. Every other file under internal/ goes through this package.
var allowedFiles = map[string]string{
	"cmd/crowbar-seed/main.go": "dev tool that refuses to default; its messages name the real home",
}

// retiredResolver matches the identifiers the old resolvers were reached by,
// plus the near-variants a sixth one would likely reuse.
var retiredResolver = regexp.MustCompile(
	`^(HomeEnvVar|Default(Crowbar)?Home(Path)?|Get(Home|StateDir|Events|Store|Config)Path(At)?)$`,
)

func TestOnlyHomeResolvesTheCrowbarHome(t *testing.T) {
	apiRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	scanned := 0
	offenders := map[string][]string{}
	walkErr := filepath.WalkDir(apiRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipDir(d.Name())
		}
		rel := filepath.ToSlash(mustRel(t, apiRoot, path))
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		scanned++
		if strings.HasPrefix(rel, "internal/core/home/") || allowedFiles[rel] != "" {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", rel, parseErr)
		}
		if reasons := resolverUses(file); len(reasons) > 0 {
			offenders[rel] = reasons
		}
		return nil
	})
	require.NoError(t, walkErr)

	require.Greater(t, scanned, 100, "a walk that scanned almost nothing proves nothing")
	t.Logf("scanned %d non-test Go files", scanned)
	for _, line := range describe(offenders) {
		t.Errorf("%s: resolve the Crowbar home through core/home", line)
	}
}

func TestResolverUses_FlagsEachShapeOfResolver(t *testing.T) {
	src := `package x
import "os"
const a = "CROWBAR_HOME"
var b = ".crowbar"
var c = "/.crowbar/x"
var d = ".crowbar/state"
var e = os.Getenv(metadata.HomeEnvVar)
var f = metadata.GetStorePath("/h")
func DefaultHome() {}
`
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)

	assert.Len(t, resolverUses(file), 7)
}

func TestResolverUses_IgnoresNearMisses(t *testing.T) {
	src := `package x
var a = ".crowbar-replace-*"
var b = "my.crowbar"
var c = "crowbar.sock"
var d = "CROWBAR_HOME is not set"
var e = metadata.GetVersion()
`
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)

	assert.Empty(t, resolverUses(file))
}

func skipDir(name string) error {
	if name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}
	return nil
}

func mustRel(t *testing.T, base string, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	require.NoError(t, err)
	return rel
}

func resolverUses(file *ast.File) []string {
	seen := map[string]bool{}
	var reasons []string
	note := func(reason string) {
		if !seen[reason] {
			seen[reason] = true
			reasons = append(reasons, reason)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING && namesTheHome(node.Value) {
				note("literal " + node.Value)
			}
		case *ast.Ident:
			if retiredResolver.MatchString(node.Name) {
				note("identifier " + node.Name)
			}
		}
		return true
	})
	return reasons
}

func namesTheHome(quoted string) bool {
	value, err := strconv.Unquote(quoted)
	if err != nil {
		return false
	}
	if value == EnvVar {
		return true
	}
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".crowbar" {
			return true
		}
	}
	return false
}

func describe(offenders map[string][]string) []string {
	files := make([]string, 0, len(offenders))
	for file := range offenders {
		files = append(files, file)
	}
	sort.Strings(files)
	lines := make([]string, 0, len(files))
	for _, file := range files {
		lines = append(lines, file+": "+strings.Join(offenders[file], ", "))
	}
	return lines
}
