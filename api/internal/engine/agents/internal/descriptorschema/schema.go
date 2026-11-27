package descriptorschema

import (
	"errors"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

const Current = 0

const VersionKey = "schema_version"

var ErrUnsupported = errors.New("descriptor schema not supported")

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
