package descriptor

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Reader reads descriptor documents of one schema version into spec.Descriptor,
// the one model everything past the loader uses.
//
// To add schema 1 while keeping 0: write a Reader for it (its own document type,
// mapped onto spec.Descriptor), register it in readers under 1, and ship the
// descriptors in it; schema 0 and every consumer stay as they are. Upgrading a
// schema 0 file to 1 is a separate step, if it is ever wanted.
type Reader interface {
	// Read parses and validates a document of this version.
	Read(raw []byte) (*spec.Descriptor, error)
	// UnknownKeys names, with their lines, the keys this version does not define.
	UnknownKeys(raw []byte) []string
}

// readers is every schema version this build reads, by the schema_version that
// selects it.
var readers = map[int]Reader{descriptorschema.Current: schema0{}}

// Parse reads raw with the reader its schema_version selects; a version with
// no reader is refused, never rewritten.
func Parse(raw []byte) (*spec.Descriptor, error) {
	r, err := readerFor(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return r.Read(raw)
}

// CheckVersion refuses a document whose schema_version selects no reader.
func CheckVersion(raw []byte) error {
	_, err := readerFor(raw)
	return err
}

// UnknownKeys are the keys raw's own schema version does not define.
func UnknownKeys(raw []byte) []string {
	r, err := readerFor(raw)
	if err != nil {
		return nil
	}
	return r.UnknownKeys(raw)
}

func readerFor(raw []byte) (Reader, error) {
	v, line, err := descriptorschema.Version(raw)
	if err != nil {
		return nil, err
	}
	if r, ok := readers[v]; ok {
		return r, nil
	}
	known := make([]string, 0, len(readers))
	for _, k := range slices.Sorted(maps.Keys(readers)) {
		known = append(known, strconv.Itoa(k))
	}
	return nil, fmt.Errorf("%w: written for schema %d (line %d), this build reads %s",
		descriptorschema.ErrUnsupported, v, line, strings.Join(known, ", "))
}

// schema0 is the pre-release schema the shipped descriptors are written in.
type schema0 struct{}

func (schema0) Read(raw []byte) (*spec.Descriptor, error) { return ParseV0(raw) }

func (schema0) UnknownKeys(raw []byte) []string {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var d spec.Descriptor
	var typeErr *yaml.TypeError
	if !errors.As(dec.Decode(&d), &typeErr) {
		return nil
	}
	return typeErr.Errors
}
