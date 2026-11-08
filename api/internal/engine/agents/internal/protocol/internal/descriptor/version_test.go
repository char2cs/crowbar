package descriptor_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// renamedTitle is a throwaway schema 1 in which display_name is spelled title:
// it maps its documents onto the same model by way of the schema 0 reader.
type renamedTitle struct{}

func (renamedTitle) Read(raw []byte) (*spec.Descriptor, error) {
	return descriptor.ParseV0(asV0(raw))
}

func (renamedTitle) UnknownKeys(raw []byte) []string { return nil }

func asV0(raw []byte) []byte {
	doc := strings.Replace(string(raw), "schema_version: 1\n", "", 1)
	return []byte(strings.Replace(doc, "\ntitle: ", "\ndisplay_name: ", 1))
}

// A new schema version is a reader registered beside the old one: both load,
// into one model, through the same Load, with nothing else changed.
func TestParse_ASecondSchemaVersionLoadsBesideTheFirst(t *testing.T) {
	descriptor.RegisterReader(t, 1, renamedTitle{})
	v0 := minimal + "display_name: Probe\n"
	v1 := "schema_version: 1\n" + minimal + "title: Probe\n"

	old, err := descriptor.Load([]byte(v0))
	require.NoError(t, err)
	next, err := descriptor.Load([]byte(v1))
	require.NoError(t, err)

	assert.Equal(t, "Probe", old.DisplayName)
	assert.Equal(t, "Probe", next.DisplayName)
}

// A version with no reader is refused, naming what this build reads.
func TestParse_AVersionWithNoReaderIsRefused(t *testing.T) {
	_, err := descriptor.Load([]byte("schema_version: 7\n" + minimal))
	require.ErrorIs(t, err, descriptorschema.ErrUnsupported)
	require.ErrorIs(t, err, descriptor.ErrInvalid)
	assert.Contains(t, err.Error(), "written for schema 7 (line 1), this build reads 0")
}
