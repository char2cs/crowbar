// Package handlers holds the fixture's route handlers: one per response shape
// protogen has to classify.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"example.com/fixture/libs"
	"example.com/fixture/types"
)

// Handlers is the fixture handler set.
type Handlers struct{}

// New builds the handler set.
func New() *Handlers {
	return &Handlers{}
}

// ListItems GET /items — a read with no request body.
func (h *Handlers) ListItems(
	c *gin.Context,
) {
	items := []types.Item{}
	libs.WriteQueryOK(c, items)
}

// GetItem GET /items/:id — a read returning one named struct.
func (h *Handlers) GetItem(
	c *gin.Context,
) {
	var item types.Item
	libs.WriteQueryOK(c, item)
}

// CreateItem POST /items — a named request body and a 201 payload.
func (h *Handlers) CreateItem(
	c *gin.Context,
) {
	var body types.CreateItemBody
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusCreated, types.Item{})
}

// RenameItem PATCH /items/:id — an anonymous request body and a mutation
// response.
func (h *Handlers) RenameItem(
	c *gin.Context,
) {
	var body struct {
		// Name is the new name.
		Name string `json:"name"`
		// Force skips the safety check.
		Force bool `json:"force,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Name)
}

// DeleteItem DELETE /items/:id — a body-less 202.
func (h *Handlers) DeleteItem(
	c *gin.Context,
) {
	libs.WriteAccepted(c)
}

// Tree GET /tree — the self-referential type.
func (h *Handlers) Tree(
	c *gin.Context,
) {
	libs.WriteQueryOK(c, types.TreeNode{})
}

// Patch GET /patch — a raw non-JSON body.
func (h *Handlers) Patch(
	c *gin.Context,
) {
	c.Data(http.StatusOK, "text/plain", nil)
}

// Untyped GET /untyped — an untyped map payload, which has no static shape.
func (h *Handlers) Untyped(
	c *gin.Context,
) {
	libs.WriteQueryOK(c, gin.H{"a": 1})
}

// Lossy GET /lossy — a payload whose type loses a field the generator cannot
// lower, which must make the endpoint report unresolved.
func (h *Handlers) Lossy(
	c *gin.Context,
) {
	libs.WriteQueryOK(c, types.Lossy{})
}

// bindPaths is the helper pattern the daemon uses: the bind happens one call
// away from the handler, so protogen has to follow it.
func bindPaths(
	c *gin.Context,
) ([]string, bool) {
	var body struct {
		// Paths is the list of repo-relative paths.
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return body.Paths, true
}

// Stage POST /stage — binds through a helper.
func (h *Handlers) Stage(
	c *gin.Context,
) {
	paths, ok := bindPaths(c)
	if !ok {
		return
	}
	libs.WriteQueryOK(c, paths)
}
