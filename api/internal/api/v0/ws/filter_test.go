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
