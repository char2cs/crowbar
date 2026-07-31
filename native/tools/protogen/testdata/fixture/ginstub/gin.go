// Package gin is a hermetic stand-in for github.com/gin-gonic/gin, carrying
// only the API surface protogen interprets: router groups, the handler type,
// the request context's bind/write methods, and gin.H.
//
// The fixture module replaces the real gin with this package so protogen's
// tests type-check offline and in a few milliseconds, while still exercising
// the exact type identities (github.com/gin-gonic/gin.RouterGroup,
// .HandlerFunc, .Context) the route walker matches on.
package gin

import "net/http"

// HandlerFunc is a request handler.
type HandlerFunc func(*Context)

// H is a shortcut for map[string]any.
type H map[string]any

// Context carries one request.
type Context struct {
	// Request is the inbound request.
	Request *http.Request
	// Writer is the response writer.
	Writer http.ResponseWriter
}

// Param returns a path parameter.
func (c *Context) Param(
	key string,
) string {
	return ""
}

// Query returns a query parameter.
func (c *Context) Query(
	key string,
) string {
	return ""
}

// DefaultQuery returns a query parameter or a fallback.
func (c *Context) DefaultQuery(
	key string,
	fallback string,
) string {
	return fallback
}

// ShouldBindJSON decodes the request body into obj.
func (c *Context) ShouldBindJSON(
	obj any,
) error {
	return nil
}

// JSON writes a JSON response.
func (c *Context) JSON(
	code int,
	obj any,
) {
}

// Data writes a raw response body.
func (c *Context) Data(
	code int,
	contentType string,
	data []byte,
) {
}

// Status writes a body-less response.
func (c *Context) Status(
	code int,
) {
}

// RouterGroup is a path-prefixed group of routes.
type RouterGroup struct {
	// basePath is the accumulated prefix.
	basePath string
}

// Group derives a nested group.
func (g *RouterGroup) Group(
	relativePath string,
	handlers ...HandlerFunc,
) *RouterGroup {
	return &RouterGroup{basePath: g.basePath + relativePath}
}

// Use installs middleware.
func (g *RouterGroup) Use(
	handlers ...HandlerFunc,
) {
}

// GET registers a GET route.
func (g *RouterGroup) GET(
	relativePath string,
	handlers ...HandlerFunc,
) {
}

// POST registers a POST route.
func (g *RouterGroup) POST(
	relativePath string,
	handlers ...HandlerFunc,
) {
}

// PUT registers a PUT route.
func (g *RouterGroup) PUT(
	relativePath string,
	handlers ...HandlerFunc,
) {
}

// PATCH registers a PATCH route.
func (g *RouterGroup) PATCH(
	relativePath string,
	handlers ...HandlerFunc,
) {
}

// DELETE registers a DELETE route.
func (g *RouterGroup) DELETE(
	relativePath string,
	handlers ...HandlerFunc,
) {
}
