//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func hermesOverSocket(t *testing.T) func(endpoint string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "internal", "engine", "agents", "internal",
		"protocol", "internal", "descriptor", "descriptors-v3", "hermes.yaml"))
	require.NoError(t, err)
	return func(endpoint string) []byte {
		doc := strings.Replace(string(raw), "connect: [hermes, gateway, attach, --json, --recover]",
			fmt.Sprintf("connect: [cat, %q]", endpoint), 1)
		require.NotEqual(t, string(raw), doc, "the descriptor's connect line moved")
		return []byte(strings.ReplaceAll(doc, "cmd: hermes", "cmd: cat"))
	}
}

func (s *socketRig) event(t *testing.T, typ, payload string) {
	t.Helper()
	s.eventAt(t, 1, typ, payload)
}

func (s *socketRig) eventAt(t *testing.T, gen int, typ, payload string) {
	t.Helper()
	s.push(t, fmt.Sprintf(`{"method":"event","params":{"type":%q,"session_id":"sess-1","admission_id":"adm-1",`+
		`"execution_generation":%d,"payload":%s}}`, typ, gen, payload))
}

func TestRegression_HermesStopInterruptsTheGenerationItsFramesName(t *testing.T) {
	s := newSocketRigFor(t, "hermes", hermesOverSocket(t))
	s.results.Store("session.create", `{"session_id":"sess-1","execution_generation":0}`)
	s.results.Store("prompt.submit", `{"admission_id":"adm-1","execution_generation":null}`)
	chatID := s.spawn("hermes")
	require.NoError(t, s.send(chatID, "run something long"))
	s.nextMethod(t, "prompt.submit")
	s.eventAt(t, 2, "message.start", `{}`)
	s.eventAt(t, 2, "tool.start", `{"tool_id":"call-1","name":"terminal","context":"sleep 60","args":{"command":"sleep 60"}}`)
	s.eventually(func() bool {
		return s.working(chatID) && len(kit.Bodies[ledger.ToolCall](s.rows(chatID), ledger.KindToolCall)) > 0
	}, "the turn's frames never landed")

	require.NoError(t, s.runners().StopChat(context.Background(), chatID))

	var params map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(s.nextMethod(t, "session.interrupt").Params, &params))
	assert.JSONEq(t, `2`, string(params["execution_generation"]), "the running turn's generation, as a number")
	assert.JSONEq(t, `"sess-1"`, string(params["session_id"]))
}

func TestRegression_StepNumberedFramesGiveEachModelCallItsOwnTextAndThought(t *testing.T) {
	s := newSocketRigFor(t, "hermes", hermesOverSocket(t))
	s.results.Store("session.create", `{"session_id":"sess-1","execution_generation":1}`)
	s.results.Store("prompt.submit", `{"admission_id":"adm-1","execution_generation":1}`)
	chatID := s.spawn("hermes")
	require.NoError(t, s.send(chatID, "run two commands"))
	s.nextMethod(t, "prompt.submit")

	s.event(t, "message.start", `{}`)
	s.event(t, "agent.step", `{"iteration":1}`)
	s.event(t, "reasoning.delta", `{"text":"first thought"}`)
	s.event(t, "message.delta", `{"text":"Running the first command.","turn_id":"t","segment":0,"offset":0}`)
	s.event(t, "message.delta", `{"text":null}`)
	s.event(t, "tool.start", `{"tool_id":"call-1","name":"terminal","context":"echo alpha","args":{"command":"echo alpha"}}`)
	s.event(t, "tool.complete", `{"tool_id":"call-1","name":"terminal","result":"alpha"}`)
	s.event(t, "agent.step", `{"iteration":2}`)
	s.event(t, "reasoning.delta", `{"text":"second thought"}`)
	s.event(t, "message.delta", `{"text":"Now the second.","turn_id":"t","segment":0,"offset":26}`)
	s.event(t, "message.delta", `{"text":null}`)
	s.event(t, "tool.start", `{"tool_id":"call-2","name":"terminal","context":"echo beta","args":{"command":"echo beta"}}`)
	s.event(t, "tool.complete", `{"tool_id":"call-2","name":"terminal","result":"beta"}`)
	s.event(t, "agent.step", `{"iteration":3}`)
	s.event(t, "message.delta", `{"text":"done","turn_id":"t","segment":0,"offset":42}`)
	s.event(t, "message.complete", `{"text":"done","status":"complete"}`)
	s.eventually(func() bool { return !s.working(chatID) && len(s.said(chatID)) > 1 }, "the turn never closed")

	rows := s.rows(chatID)
	var ids, texts []string
	for _, a := range kit.Bodies[ledger.AssistantText](rows, ledger.KindAssistantText) {
		ids, texts = append(ids, a.MessageID), append(texts, a.Text)
	}
	assert.Equal(t, []string{"Running the first command.", "Now the second.", "done"}, texts)
	assert.Len(t, slices.Compact(slices.Sorted(slices.Values(ids))), 3,
		"the text before a tool call and the text after it are different messages")

	thoughts := kit.Bodies[ledger.Reasoning](rows, ledger.KindReasoning)
	require.Len(t, thoughts, 2, "each thought is its own row, never the second appended to the first")
	assert.Equal(t, []string{"first thought", "second thought"}, []string{thoughts[0].Text, thoughts[1].Text})
	assert.Equal(t, ids[0], thoughts[0].MessageID, "a thought belongs to its step's text")
	assert.Equal(t, ids[1], thoughts[1].MessageID)
}

type relayEnv struct{ Name, Value string }

type relayServer struct {
	Name    string     `json:"name"`
	Command string     `json:"command"`
	Args    []string   `json:"args"`
	Env     []relayEnv `json:"env"`
}

type sessionCall struct {
	Source    string `json:"source"`
	SessionID string `json:"session_id"`
	Editor    struct {
		MCPServers []relayServer `json:"mcp_servers"`
	} `json:"editor"`
	Raw map[string]json.RawMessage `json:"-"`
}

func decodeSessionCall(t *testing.T, f wireMsg) sessionCall {
	t.Helper()
	var c sessionCall
	require.NoError(t, json.Unmarshal(f.Params, &c))
	require.NoError(t, json.Unmarshal(f.Params, &c.Raw))
	require.Len(t, c.Editor.MCPServers, 1, "the call carries no relay: %s", f.Params)
	return c
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	i := slices.Index(args, flag)
	require.GreaterOrEqual(t, i, 0, "%s missing from %v", flag, args)
	require.Less(t, i+1, len(args))
	return args[i+1]
}

func readToken(t *testing.T, srv relayServer) string {
	t.Helper()
	path := flagValue(t, srv.Args, "--token-file")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	token, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	return string(token)
}

func (s *socketRig) firstTurn(t *testing.T, chatID string) {
	t.Helper()
	require.NoError(t, s.send(chatID, "hi"))
	s.nextMethod(t, "prompt.submit")
	s.event(t, "message.start", `{}`)
	s.event(t, "message.complete", `{"text":"hello","status":"complete"}`)
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: hello") && !s.working(chatID) },
		"the first turn never closed")
}

func (s *socketRig) relayAnswer(t *testing.T, srv relayServer) string {
	t.Helper()
	out, send, err := s.d.app.Usecases.AgentProvider.DispatchRelayMCP(context.Background(),
		flagValue(t, srv.Args, "--relay"), readToken(t, srv),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_workspaces","arguments":{}}}`))
	require.NoError(t, err)
	require.True(t, send)
	return string(out)
}

func TestRegression_HermesSessionCallsHandTheGatewayCrowbarsMCPRelay(t *testing.T) {
	s := newSocketRigFor(t, "hermes", hermesOverSocket(t))
	s.results.Store("session.create", `{"session_id":"sess-1","execution_generation":1}`)
	s.results.Store("prompt.submit", `{"admission_id":"adm-1","execution_generation":1}`)
	chatID := s.spawn("hermes")

	createFrame := s.nextMethod(t, "session.create")
	create := decodeSessionCall(t, createFrame)
	assert.Equal(t, "acp", create.Source)
	assert.NotContains(t, create.Raw, "toolsets", "acp defaults, no toolsets override")
	relay := create.Editor.MCPServers[0]
	assert.Equal(t, "crowbar", relay.Name)
	assert.Equal(t, crowbarBin, relay.Command)
	require.NotEmpty(t, relay.Args)
	assert.Equal(t, "mcp", relay.Args[0])
	for _, flag := range []string{"--relay", "--token-file", "--host"} {
		assert.Contains(t, relay.Args, flag)
	}
	for _, flag := range []string{"--segment", "--workspace", "--project", "--repo"} {
		assert.NotContains(t, relay.Args, flag, "the gateway pins this block: nothing a move or a new runner changes rides it")
	}
	require.Len(t, relay.Env, 1)
	assert.Equal(t, "CROWBAR_HOME", relay.Env[0].Name, "an MCP server may get a scrubbed env: the relay is told its home")
	assert.NotEmpty(t, relay.Env[0].Value)
	assert.NotContains(t, string(createFrame.Params), readToken(t, relay), "a request body is logged")

	s.firstTurn(t, chatID)
	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)

	require.NoError(t, s.send(chatID, "again"))
	resumeFrame := s.nextMethod(t, "session.resume")
	resume := decodeSessionCall(t, resumeFrame)
	assert.Equal(t, "sess-1", resume.SessionID)
	assert.Equal(t, relay, resume.Editor.MCPServers[0],
		"a restarted gateway has no memory of the relay: resume re-sends the block the session was created with")
	assert.NotContains(t, string(resumeFrame.Params), readToken(t, resume.Editor.MCPServers[0]))
}

func TestRegression_HermesSecondRunnerResumesWithTheIdenticalBlock(t *testing.T) {
	s := newSocketRigFor(t, "hermes", hermesOverSocket(t))
	s.results.Store("session.create", `{"session_id":"sess-1","execution_generation":1}`)
	s.results.Store("prompt.submit", `{"admission_id":"adm-1","execution_generation":1}`)
	chatID := s.spawn("hermes")
	createFrame := s.nextMethod(t, "session.create")
	relay := decodeSessionCall(t, createFrame).Editor.MCPServers[0]
	_, firstRunner := s.live(chatID)
	s.firstTurn(t, chatID)
	assert.NotContains(t, s.relayAnswer(t, relay), "unauthorized", "the live runner's session is served")

	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	assert.Contains(t, s.relayAnswer(t, relay), "unauthorized", "a relay with no live runner behind it is refused")
	s.offerEndpoint(t)

	require.NoError(t, s.send(chatID, "again"))
	resumeFrame := s.nextMethod(t, "session.resume")
	var created, resumed struct {
		Editor json.RawMessage `json:"editor"`
	}
	require.NoError(t, json.Unmarshal(createFrame.Params, &created))
	require.NoError(t, json.Unmarshal(resumeFrame.Params, &resumed))
	assert.JSONEq(t, string(created.Editor), string(resumed.Editor))
	_, secondRunner := s.live(chatID)
	assert.NotEqual(t, firstRunner, secondRunner, "the chat's second runner is what resumed")
	assert.NotContains(t, s.relayAnswer(t, relay), "unauthorized", "the same relay now reaches the second runner")
}
