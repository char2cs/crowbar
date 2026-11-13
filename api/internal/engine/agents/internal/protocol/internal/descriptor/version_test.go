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

const httpDelivery = `id: probe
spawn: { cmd: probe-cli, interactive_required: true }
runtime:
  transport: api
  api:
    delivery: http
    format: sse
    connect: [agent-cli, endpoint]
    connect_map: { url: base, credential: token, headers: { X-Trace: trace } }
    auth: { bearer_path: credential }
    requests:
      create: { method: POST, path: /runs, body: { input: "{prompt}" }, capture: { run_id: id }, ok_status: [200, 201] }
      stop: { method: POST, path: "/runs/{run_id}/stop" }
    stream: { path: "/runs/{run_id}/events" }
    reconnect: { max: 3, backoff_ms: 500 }
`

func TestLoad_ReadsAnHTTPDeliveryAndRefusesAnInvalidOne(t *testing.T) {
	d, err := descriptor.Load([]byte(httpDelivery))
	require.NoError(t, err)
	assert.Equal(t, "/runs/{run_id}/events", d.Runtime.API.Stream.Path)
	assert.Equal(t, "id", d.Runtime.API.Requests["create"].Capture["run_id"])
	assert.Equal(t, 500, d.Runtime.API.Reconnect.BackoffMS)

	_, err = descriptor.Load([]byte(strings.Replace(httpDelivery, "format: sse", "format: ndjson", 1)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime.api.format")
}
