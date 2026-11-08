// Package descriptorschema is how a descriptor document says which schema it
// is written in. The schema is pre-release and changes without migration: a
// document is read by the reader its version selects, or refused, never rewritten.
package descriptorschema

import (
	"errors"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Current is the schema the shipped descriptors are written in; a document that
// states none is in it.
const Current = 0

// VersionKey is the top-level key a document states its schema in.
const VersionKey = "schema_version"

// ErrUnsupported is a document stating a schema this build has no reader for.
var ErrUnsupported = errors.New("descriptor schema not supported")

// Version is the schema raw states and the line it states it on: Current when
// it states none, or does not parse (that is the parser's to report).
func Version(raw []byte) (version, line int, err error) {
	var doc struct {
		Version yaml.Node `yaml:"schema_version"`
	}
	if parseErr := yaml.Unmarshal(raw, &doc); parseErr != nil {
		return Current, 0, nil //nolint:nilerr // a document that does not parse is the parser's to report, in its own reader
	}
	if doc.Version.Kind == 0 {
		return Current, 0, nil
	}
	v, err := strconv.Atoi(doc.Version.Value)
	if err != nil {
		return 0, doc.Version.Line, fmt.Errorf("%w: %s %q (line %d) is not a schema number",
			ErrUnsupported, VersionKey, doc.Version.Value, doc.Version.Line)
	}
	return v, doc.Version.Line, nil
}
