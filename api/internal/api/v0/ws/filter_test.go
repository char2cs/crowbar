package ws

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type row struct {
	name string
	kind string
}

func ctxWith(
	query string,
	ns string,
) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
	if ns != "" {
		c.Params = gin.Params{{Key: "ns", Value: ns}}
	}
	return c
}

func def() StreamDef[row] {
	return StreamDef[row]{
		Namespace: func(r row) string { return r.name },
		Filters: []FilterDef[row]{
			{Param: "kind", Extract: func(r row) string { return r.kind }, Match: ExactMatch},
		},
	}
}

func TestBuildPredicate_FilterMatches(t *testing.T) {
	p := BuildPredicate(ctxWith("kind=fruit", ""), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestBuildPredicate_NamespaceGlob(t *testing.T) {
	p := BuildPredicate(ctxWith("", "al*"), def())
	assert.True(t, p(row{name: "alpha"}))
	assert.False(t, p(row{name: "beta"}))
}

func TestBuildPredicate_NoFiltersMatchesAll(t *testing.T) {
	p := BuildPredicate(ctxWith("", ""), StreamDef[row]{Namespace: func(r row) string { return r.name }})
	assert.True(t, p(row{name: "anything"}))
}

func TestGlobMatch_EmptyPatternMatchesAll(t *testing.T) {
	assert.True(t, GlobMatch("", "x"))
}

func TestExactMatch(t *testing.T) {
	assert.True(t, ExactMatch("a", "a"))
	assert.False(t, ExactMatch("a", "b"))
}

func TestGlobMatch_InvalidPattern_NoMatch(t *testing.T) {
	assert.False(t, GlobMatch("[invalid", "anything"))
}

func TestBuildPredicate_DefaultFilter(t *testing.T) {
	d := StreamDef[row]{
		Namespace: func(r row) string { return r.name },
		Filters: []FilterDef[row]{
			{Param: "kind", Extract: func(r row) string { return r.kind }, Match: ExactMatch, Default: "fruit"},
		},
	}
	p := BuildPredicate(ctxWith("", ""), d)
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

// TestBuildPredicate_DeclaredFilterWithNoValueAnywhereMatchesAll proves a
// declared Filter with no Default degrades to "matches everything" when the
// request carries its Param in neither the path nor the query — the case a
// stream's route hits once it is mounted somewhere that no longer binds that
// path param at all (e.g. the agent-chat WS's wsId Filter once its route
// moved off .../workspaces/:wsId, Task 17), rather than refusing every event.
func TestBuildPredicate_DeclaredFilterWithNoValueAnywhereMatchesAll(t *testing.T) {
	p := BuildPredicate(ctxWith("", ""), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.True(t, p(row{name: "carrot", kind: "veg"}))
}

func TestGlobMatch_Wildcard(t *testing.T) {
	assert.True(t, GlobMatch("al*", "alpha"))
	assert.False(t, GlobMatch("al*", "beta"))
}

func TestMatchesAll_EmptyActive_ReturnsTrue(t *testing.T) {
	result := matchesAll([]activeFilter[row]{}, row{name: "x", kind: "y"})
	assert.True(t, result)
}

func ctxWithParam(
	query string,
	key string,
	value string,
) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
	c.Params = gin.Params{{Key: key, Value: value}}
	return c
}

func TestBuildPredicate_FilterFromQueryParam(t *testing.T) {
	p := BuildPredicate(ctxWith("kind=fruit", ""), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestBuildPredicate_FilterFromPathParam(t *testing.T) {
	p := BuildPredicate(ctxWithParam("", "kind", "fruit"), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestBuildPredicate_PathParamWinsOverQuery(t *testing.T) {
	p := BuildPredicate(ctxWithParam("kind=veg", "kind", "fruit"), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestBuildPredicate_FallsBackToQueryWhenPathEmpty(t *testing.T) {
	p := BuildPredicate(ctxWithParam("kind=fruit", "other", "x"), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestPrefixMatch_Exact(t *testing.T) {
	assert.True(t, PrefixMatch("p/r/w", "p/r/w"))
}

func TestPrefixMatch_ParentPrefixMatchesChild(t *testing.T) {
	assert.True(t, PrefixMatch("p/r", "p/r/w"))
	assert.True(t, PrefixMatch("p/r", "p/r"))
}

func TestPrefixMatch_RejectsSiblingRepo(t *testing.T) {
	assert.False(t, PrefixMatch("p/r", "p/r2/w"))
}

func TestPrefixMatch_RejectsPartialSegment(t *testing.T) {
	assert.False(t, PrefixMatch("p/r", "p/rx"))
}

func TestPrefixMatch_ProjectPrefix(t *testing.T) {
	assert.True(t, PrefixMatch("p", "p/r/w"))
}

func TestPrefixMatch_EmptyMatchesAll(t *testing.T) {
	assert.True(t, PrefixMatch("", "p/r/w"))
	assert.True(t, PrefixMatch("", ""))
	assert.True(t, PrefixMatch("", "anything"))
}

// ctxWithParams builds a context carrying multiple path params, mirroring a
// request to a route nested under /projects/:projectId/repos/:repoId.
func ctxWithParams(
	query string,
	params gin.Params,
) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
	c.Params = params
	return c
}

// flatDef mirrors the git/files/lsp streams: a bare wsId namespace, a wsId
// Filter, and FlatNamespace set.
func flatDef() StreamDef[row] {
	return StreamDef[row]{
		Namespace:     func(r row) string { return r.name },
		FlatNamespace: true,
		Filters: []FilterDef[row]{
			{Param: "wsId", Extract: func(r row) string { return r.name }, Match: ExactMatch},
		},
	}
}

// TestBuildPredicate_FlatNamespace_IgnoresHierarchicalScope proves a
// flat-namespace stream subscribed via a nested route (projectId/repoId present
// as path params) is scoped ONLY by its wsId Filter: the structural
// projectId/repoId would otherwise build a "p1/r1" client scope that can never
// prefix-match the bare wsId namespace and would drop every event.
func TestBuildPredicate_FlatNamespace_IgnoresHierarchicalScope(t *testing.T) {
	ctx := ctxWithParams("wsId=A", gin.Params{
		{Key: "projectId", Value: "p1"},
		{Key: "repoId", Value: "r1"},
	})
	p := BuildPredicate(ctx, flatDef())
	assert.True(t, p(row{name: "A"}), "matching wsId must pass despite the p1/r1 path")
	assert.False(t, p(row{name: "B"}), "non-matching wsId is filtered by the Filter")
}

// TestBuildPredicate_FlatNamespace_DualServePathParam proves the dual-serve
// route (which binds wsId as a PATH param under the nested prefix) still scopes
// by wsId for a flat-namespace stream.
func TestBuildPredicate_FlatNamespace_DualServePathParam(t *testing.T) {
	ctx := ctxWithParams("", gin.Params{
		{Key: "projectId", Value: "p1"},
		{Key: "repoId", Value: "r1"},
		{Key: "wsId", Value: "A"},
	})
	p := BuildPredicate(ctx, flatDef())
	assert.True(t, p(row{name: "A"}))
	assert.False(t, p(row{name: "B"}))
}
