package ws

import (
	"path"

	"github.com/gin-gonic/gin"
)

// ExactMatch reports whether param equals value.
func ExactMatch(
	param string,
	value string,
) bool {
	return param == value
}

// GlobMatch reports whether value matches the glob pattern ("" matches all).
func GlobMatch(
	pattern string,
	value string,
) bool {
	if pattern == "" {
		return true
	}
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}

type activeFilter[T any] struct {
	param string
	fd    FilterDef[T]
}

// BuildPredicate compiles the namespace glob and active query-param filters from
// the request into a single predicate over T.
func BuildPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
) func(T) bool {
	nsPattern := c.Param("ns")
	active := collectFilters(c, def)
	return func(event T) bool {
		if nsPattern != "" && !GlobMatch(nsPattern, def.Namespace(event)) {
			return false
		}
		return matchesAll(active, event)
	}
}

func collectFilters[T any](
	c *gin.Context,
	def StreamDef[T],
) []activeFilter[T] {
	var active []activeFilter[T]
	for _, f := range def.Filters {
		v := resolveFilterValue(c, f)
		if v != "" {
			active = append(active, activeFilter[T]{param: v, fd: f})
		}
	}
	return active
}

// resolveFilterValue reads a filter Param from the PATH param first, falling
// back to the QUERY param, then the FilterDef Default. The path-first order lets
// a single FilterDef scope both the dual-served path route
// (/v0/workspaces/:wsId/git/status) and the dedicated query route
// (/v0/ws/git?wsId=).
func resolveFilterValue[T any](
	c *gin.Context,
	f FilterDef[T],
) string {
	if v := c.Param(f.Param); v != "" {
		return v
	}
	if v := c.Query(f.Param); v != "" {
		return v
	}
	return f.Default
}

func matchesAll[T any](
	active []activeFilter[T],
	event T,
) bool {
	for _, af := range active {
		if !af.fd.Match(af.param, af.fd.Extract(event)) {
			return false
		}
	}
	return true
}
