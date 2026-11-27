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

type Reader interface {
	Read(raw []byte) (*spec.Descriptor, error)
	UnknownKeys(raw []byte) []string
}

var readers = map[int]Reader{descriptorschema.Current: schema0{}}

func Parse(raw []byte) (*spec.Descriptor, error) {
	r, err := readerFor(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return r.Read(raw)
}

func CheckVersion(raw []byte) error {
	_, err := readerFor(raw)
	return err
}

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
