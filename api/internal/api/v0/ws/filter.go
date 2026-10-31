package ws

import (
	"path"
	"strings"

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

// PrefixMatch reports whether prefix is a HIERARCHICAL segment-prefix of value.
// Both are split on "/"; prefix matches value iff every prefix segment equals the
// segment at the same position in value, and value has at least as many segments.
// So "p/r" matches "p/r/w" and "p/r" but NOT "p/r2/w" and NOT "p/rx"; "p" matches
// "p/r/w"; the empty prefix matches every value.
func PrefixMatch(
	prefix string,
	value string,
) bool {
	if prefix == "" {
		return true
	}
	prefixSegs := strings.Split(prefix, "/")
	valueSegs := strings.Split(value, "/")
	if len(prefixSegs) > len(valueSegs) {
		return false
	}
	for i, seg := range prefixSegs {
		if seg != valueSegs[i] {
			return false
		}
	}
	return true
}

// clientScope derives the connecting client's scope from the request.
//
// The hierarchical form joins the projectId/repoId/wsId path params (falling
// back to query params) into "p/r/w". Trailing empty segments are trimmed, so a
// repo-level subscription yields "p/r" and a project-level one yields "p".
//
// A CHAT-scoped route (/v0/chats/:chatId/...) binds none of those three, so the
// scope is the bare chat id instead — the flat, single-segment form the
// chat-keyed streams' FlatNamespace already uses (spec §7.1). It is read only
// as a fallback, after the hierarchical segments have all come back empty, so a
// route that binds both keeps its hierarchical scope unchanged.
//
// When neither shape is present it returns "" (matches all).
func clientScope(
	c *gin.Context,
) string {
	segs := []string{
		scopeParam(c, "projectId"),
		scopeParam(c, "repoId"),
		scopeParam(c, "wsId"),
	}
	end := len(segs)
	for end > 0 && segs[end-1] == "" {
		end--
	}
	if end == 0 {
		return scopeParam(c, "chatId")
	}
	return strings.Join(segs[:end], "/")
}

func scopeParam(
	c *gin.Context,
	name string,
) string {
	if v := c.Param(name); v != "" {
		return v
	}
	return c.Query(name)
}

type activeFilter[T any] struct {
	param string
	fd    FilterDef[T]
}

// BuildPredicate compiles the client's namespace match and active query-param
// filters from the request into a single predicate over T.
//
// Namespace scoping is hierarchical when the request carries scoping path params
// (projectId/repoId/wsId): the client's scope prefix (e.g. "p/r" for a repo-level
// subscription) is PrefixMatched against each event namespace, so a repo-scoped
// client receives every child workspace's events ("p/r" matches "p/r/w"). When no
// scoping params are present, BuildPredicate falls back to the legacy ":ns" glob
// route, so existing glob subscriptions are unchanged.
//
// FlatNamespace streams opt out of the hierarchical prefix: the structural
// projectId/repoId path params would build a "p/r" prefix that can never match
// a bare-id namespace. They are scoped solely by their explicit Filters.
func BuildPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
) func(T) bool {
	active, unscoped := collectFilters(c, def)
	if unscoped {
		return func(T) bool { return false }
	}
	return scopedPredicate(c, def, active)
}

func scopedPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
	active []activeFilter[T],
) func(T) bool {
	if def.FlatNamespace {
		return func(event T) bool {
			return matchesAll(active, event)
		}
	}
	scope := clientScope(c)
	nsPattern := c.Param("ns")
	return func(event T) bool {
		switch {
		case scope != "":
			if !PrefixMatch(scope, def.Namespace(event)) {
				return false
			}
		case nsPattern != "" && !GlobMatch(nsPattern, def.Namespace(event)):
			return false
		}
		return matchesAll(active, event)
	}
}

// collectFilters compiles the client's active filters, reporting whether the
// request left a Required filter with nothing to scope by — the one case where
// the predicate must match NOTHING rather than everything.
func collectFilters[T any](
	c *gin.Context,
	def StreamDef[T],
) ([]activeFilter[T], bool) {
	var active []activeFilter[T]
	for _, f := range def.Filters {
		v := resolveFilterValue(c, f)
		if v == "" && f.Required {
			return nil, true
		}
		if v == "" {
			continue
		}
		active = append(active, activeFilter[T]{param: v, fd: f})
	}
	return active, false
}

// resolveFilterValue reads a filter Param from the PATH param first, falling
// back to the QUERY param, then the FilterDef Default, so one FilterDef scopes
// a path-param mount and a ?param= caller alike.
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
		if !af.matches(event) {
			return false
		}
	}
	return true
}

func (a activeFilter[T]) matches(
	event T,
) bool {
	return a.fd.Match(a.param, a.fd.Extract(event))
}
