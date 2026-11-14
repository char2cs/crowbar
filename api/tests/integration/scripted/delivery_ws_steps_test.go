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

// hermesOverSocket is the shipped hermes descriptor with only its connection swapped
// for the rig's fake socket, so the events under test are the real mappings.
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

// event pushes one frame of a prompt in the envelope the gateway wraps every event in.
func (s *socketRig) event(t *testing.T, typ, payload string) {
	t.Helper()
	s.push(t, fmt.Sprintf(`{"method":"event","params":{"type":%q,"session_id":"sess-1","admission_id":"adm-1","payload":%s}}`,
		typ, payload))
}

// A gateway that numbers its model calls and ids no text or thought of its own: each
// call's text and thought is one block, and the blocks keep the order the tool rows
// were said in, not one stream of everything the prompt said.
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

// sessionCall is the part of a session.create/resume body this suite reads; Raw keeps what must be absent.
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

// flagValue is the argument after flag in args.
func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	i := slices.Index(args, flag)
	require.GreaterOrEqual(t, i, 0, "%s missing from %v", flag, args)
	require.Less(t, i+1, len(args))
	return args[i+1]
}

// withoutRunnerIdentity is srv with the values that name one runner blanked.
func withoutRunnerIdentity(t *testing.T, srv relayServer) relayServer {
	t.Helper()
	args := slices.Clone(srv.Args)
	for _, flag := range []string{"--segment", "--token-file"} {
		flagValue(t, args, flag)
		args[slices.Index(args, flag)+1] = ""
	}
	srv.Args = args
	return srv
}

// readToken is the token file a relay reads, which must be 0600 and hold something.
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

// Hermes has one per-session MCP path, and the gateway keeps it in memory only: the create
// hands Crowbar's relay over as an ACP stdio server, and a resume after the connection is
// gone hands it over again. The token rides a 0600 file, never the body.
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
	for _, flag := range []string{"--segment", "--token-file", "--project", "--workspace", "--repo", "--host"} {
		assert.Contains(t, relay.Args, flag)
	}
	require.Len(t, relay.Env, 1)
	assert.Equal(t, "CROWBAR_HOME", relay.Env[0].Name, "an MCP server may get a scrubbed env: the relay is told its home")
	assert.NotEmpty(t, relay.Env[0].Value)
	assert.NotContains(t, string(createFrame.Params), readToken(t, relay), "a request body is logged")

	require.NoError(t, s.send(chatID, "hi"))
	s.nextMethod(t, "prompt.submit")
	s.event(t, "message.start", `{}`)
	s.event(t, "message.complete", `{"text":"hello","status":"complete"}`)
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: hello") && !s.working(chatID) },
		"the first turn never closed")
	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)

	require.NoError(t, s.send(chatID, "again"))
	resumeFrame := s.nextMethod(t, "session.resume")
	resume := decodeSessionCall(t, resumeFrame)
	assert.Equal(t, "sess-1", resume.SessionID)
	// A revived chat is a new runner: the same block, naming that runner's own segment and token.
	resumed := resume.Editor.MCPServers[0]
	assert.Equal(t, withoutRunnerIdentity(t, relay), withoutRunnerIdentity(t, resumed),
		"a restarted gateway has no memory of the relay: resume re-sends it")
	assert.NotEqual(t, flagValue(t, relay.Args, "--segment"), flagValue(t, resumed.Args, "--segment"))
	assert.NotContains(t, string(resumeFrame.Params), readToken(t, resumed))
}
