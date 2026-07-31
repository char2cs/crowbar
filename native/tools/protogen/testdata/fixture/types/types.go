// Package types holds the DTO shapes protogen's tests assert on: json tags,
// omitempty, pointers, embedded structs, slices, maps, nested structs, named
// string enums, json:"-", byte slices, time.Time, raw JSON, and a
// self-referential type.
package types

import (
	"encoding/json"
	"time"
)

// Status is a named string type with a package-level constant set, which is
// what makes it an enum rather than a bare string alias.
type Status string

const (
	// StatusNew is a freshly created item.
	StatusNew Status = "new"
	// StatusLocked is an item held elsewhere.
	StatusLocked Status = "locked"
	// StatusPRConflicts is an item whose merge would conflict.
	StatusPRConflicts Status = "pr-conflicts"
)

// Untagged is a named string type with no constants: an alias, not an enum.
type Untagged string

// Base is embedded into Item without a json tag, so encoding/json promotes its
// fields into the parent object rather than nesting them.
type Base struct {
	// ID is the entity id.
	ID string `json:"id"`
	// CreatedAt is when it was created.
	CreatedAt time.Time `json:"createdAt"`
}

// Meta is embedded into Item WITH a json tag, so it stays a nested object.
type Meta struct {
	// Author is who made the change.
	Author string `json:"author"`
}

// Nested is a plain struct used as a field type.
type Nested struct {
	// Label is the display label.
	Label string `json:"label"`
	// Count is a counter.
	Count int `json:"count"`
}

// Item exercises every field shape the generator has to lower.
type Item struct {
	Base
	// Meta is embedded but tagged, so it nests.
	Meta `json:"meta"`
	// Name is a plain required string.
	Name string `json:"name"`
	// Description is omitted when empty.
	Description string `json:"description,omitempty"`
	// Parent is a pointer, so it can be null or absent.
	Parent *string `json:"parent,omitempty"`
	// Child is a pointer to a struct with no omitempty: null, never absent.
	Child *Nested `json:"child"`
	// Tags is a slice; a nil slice marshals as null.
	Tags []string `json:"tags"`
	// Items is a slice of structs, omitted when empty.
	Items []Nested `json:"items,omitempty"`
	// Counts is a map of ints.
	Counts map[string]int `json:"counts"`
	// Extra is an untyped map: arbitrary JSON.
	Extra map[string]any `json:"extra,omitempty"`
	// Raw is a raw JSON message.
	Raw json.RawMessage `json:"raw,omitempty"`
	// Blob is a byte slice, which encoding/json base64-encodes to a string.
	Blob []byte `json:"blob,omitempty"`
	// Status is the enum.
	Status Status `json:"status,omitempty"`
	// Kind is a named string with no constants.
	Kind Untagged `json:"kind"`
	// Type collides with a Rust keyword and forces an explicit rename.
	Type string `json:"type"`
	// Internal never reaches the wire.
	Internal string `json:"-"`
	// unexported is invisible to encoding/json.
	unexported string
	// Size is an int64: a JSON number, not a string.
	Size int64 `json:"size"`
	// Ratio is a float.
	Ratio float64 `json:"ratio"`
	// Done is a bool omitted when false.
	Done bool `json:"done,omitempty"`
}

// TreeNode is self-referential, which the transitive closure must not recurse
// into forever.
type TreeNode struct {
	// Name is the node name.
	Name string `json:"name"`
	// Children are the child nodes.
	Children []TreeNode `json:"children"`
}

// Lossy carries a field the generator cannot lower: an anonymous struct nested
// inside a slice has no name to emit, so the field is dropped and the type is
// reported INCOMPLETE rather than quietly shipped a field short.
type Lossy struct {
	// Name is fine.
	Name string `json:"name"`
	// Rows is a slice of anonymous structs, which has no emittable name.
	Rows []struct {
		// Value is unreachable from the generated types.
		Value string `json:"value"`
	} `json:"rows"`
}

// CreateItemBody is a named request body.
type CreateItemBody struct {
	// Name is required.
	Name string `json:"name"`
	// Status is optional.
	Status Status `json:"status,omitempty"`
}

// use keeps the unexported field referenced so the fixture compiles cleanly.
func (i Item) use() string {
	return i.unexported
}
