package agents_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func dialedAgent(t *testing.T, connect string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	writeDescriptor(t, home, "dialed", `
id: dialed
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: rest
    delivery: http
    format: sse
    connect: `+connect+`
    connect_map: { url: url }
    requests:
      create: { method: POST, path: /runs, capture: { run_id: id } }
    stream: { path: "/runs/{run_id}/events" }
`)
	a, err := agents.New().Get(context.Background(), home, "dialed")
	require.NoError(t, err)
	return a
}

func TestAgent_APIDialedIsTrueOnlyForADescriptorThatDialsItsServer(t *testing.T) {
	assert.True(t, dialedAgent(t, `[sh]`).APIDialed())
	assert.False(t, get(t, "claude").APIDialed())
}

func TestAgent_StartAPIEndpointRunsTheConnectCommandAndParsesItsStdout(t *testing.T) {
	a := dialedAgent(t, `[sh, -c, 'echo "{\"url\":\"http://127.0.0.1:1\"}"']`)

	conn, err := a.StartAPIEndpoint(t.Context(), agents.TemplateCtx{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestAgent_StartAPIEndpointReportsAConnectCommandThatPrintsNothingUsable(t *testing.T) {
	a := dialedAgent(t, `[sh, -c, "echo not-json"]`)

	_, err := a.StartAPIEndpoint(t.Context(), agents.TemplateCtx{}, nil)

	require.ErrorContains(t, err, "not a JSON object")
}

func TestAgent_StartAPIEndpointReportsAConnectCommandThatFails(t *testing.T) {
	a := dialedAgent(t, `[sh, -c, "exit 3"]`)

	_, err := a.StartAPIEndpoint(t.Context(), agents.TemplateCtx{}, nil)

	require.ErrorContains(t, err, "connect")
}
