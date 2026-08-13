package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// mcpResult decodes a JSON-RPC response's result member, failing the test on a
// JSON-RPC error (which is a TRANSPORT fault here — a tool that merely failed
// comes back as a successful result carrying isError).
func mcpResult(
	t *testing.T,
	raw []byte,
	into any,
) {
	t.Helper()
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error, "unexpected JSON-RPC error: %s", resp.Error)
	require.NoError(t, json.Unmarshal(resp.Result, into))
}

type mcpCallResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// TestDispatchMCP_InitializeAnnouncesCrowbar proves the seam builds a server
// that speaks the pinned protocol revision and identifies itself as crowbar —
// the handshake every vendor CLI's MCP client performs before it will call a
// tool.
func TestDispatchMCP_InitializeAnnouncesCrowbar(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	mcpResult(t, out, &res)
	assert.Equal(t, enginemcp.ProtocolVersion, res.ProtocolVersion)
	assert.Equal(t, "crowbar", res.ServerInfo.Name)
}

// TestDispatchMCP_ListsTheChatTools proves the usecase wires itself in as BOTH
// the tool surface's ChatRenamer (Deps.Chats) and its ChatLogReader
// (Deps.ChatLogs). agenttools registers nothing for a nil dependency, so a
// build that failed to make either self-assignment would still answer
// tools/list successfully — just with set_chat_title or get_chat_log quietly
// missing — and nothing else would ever notice.
//
// This asserts the FULL 8-name production surface, not merely Contains, and it
// does so against a REAL *agent.Usecase built through the real agent.New (via
// newFixture → newFixtureUsing), never a Deps struct a test hand-assembles and
// then patches — the latter can only prove the surface WOULD be complete if
// New's own self-assignment ran, not that it actually did. That distinction is
// exactly what a deleted `u.tools.Chats = u` or `u.tools.ChatLogs = u` needs to
// be caught by something.
func TestDispatchMCP_ListsTheChatTools(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mcpResult(t, out, &res)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"set_chat_title",
		"list_review_threads",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "a real *agent.Usecase must advertise the complete production tool surface")
}

// TestDispatchMCP_ToolCallRenamesTheCallersChat is the end-to-end proof of the
// seam: an authenticated tools/call reaches a handler, and that handler's effect
// lands on the chat the CALLING runner is on right now — resolved from the
// runner, never from an argument, which is why no tool takes a chat id.
func TestDispatchMCP_ToolCallRenamesTheCallersChat(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Fix The Parser"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "tool reported an error: %+v", res.Content)

	assert.Equal(t, "Fix The Parser", f.chat(t, chatID).Title)
}

// TestDispatchMCP_RejectsAnotherRunnersToken is the authority test: holding a
// token minted for a DIFFERENT runner must not let a caller act as this one. The
// refusal comes back as a tool error rather than a transport error — that is how
// the model is told — but the write must not happen.
func TestDispatchMCP_RejectsAnotherRunnersToken(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, otherRunnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(otherRunnerID),
		[]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Stolen"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	assert.True(t, res.IsError, "a token minted for another runner must not authenticate")
	assert.Empty(t, f.chat(t, chatID).Title, "the rejected call must not have written")
}

// TestDispatchMCP_NotificationIsSilent proves a JSON-RPC notification produces
// no reply at all, which the HTTP seam turns into 204.
func TestDispatchMCP_NotificationIsSilent(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	require.NoError(t, err)
	assert.False(t, send)
	assert.Empty(t, out)
}

// TestDispatchMCP_UnconfiguredSurfaceIsAnError proves a usecase built without a
// tool surface REFUSES the call instead of answering with an empty tool list. A
// wiring mistake has to be loud: an agent that silently has no tools looks
// identical to an agent that chose not to use them.
func TestDispatchMCP_UnconfiguredSurfaceIsAnError(
	t *testing.T,
) {
	u := agentusecase.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, agenttools.Deps{})

	_, send, err := u.DispatchMCP(t.Context(), "seg-1", "tok",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.Error(t, err)
	assert.False(t, send)
}

// TestSpawnChat_MintsARunnerTokenTheDispatchSeamAccepts closes the loop the
// descriptor tests can only render half of: the token baked into the CLI's argv at
// spawn must be one THIS daemon's minter verifies. A spawn path that minted from a
// different secret — or shipped the segment id twice — would render argv that looks
// perfectly right and authenticate nothing.
func TestSpawnChat_MintsARunnerTokenTheDispatchSeamAccepts(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	var config struct {
		MCPServers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw := argAfter(t, f.term.calls[0].argv, "--mcp-config")
	require.NoError(t, json.Unmarshal([]byte(raw), &config), "rendered --mcp-config is not valid JSON: %s", raw)

	args := config.MCPServers["crowbar"].Args
	require.Equal(t, runnerID, args[indexOf(args, "--segment")+1])
	token := args[indexOf(args, "--token")+1]
	require.NotEmpty(t, token)
	require.NotEqual(t, runnerID, token, "the token must not be the segment id the agent can already read")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, token,
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Minted At Spawn"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "the token the CLI was actually given must authenticate: %+v", res.Content)
	assert.Equal(t, "Minted At Spawn", f.chat(t, chatID).Title)
}

// TestSpawnChat_InjectsTheCapabilityPreamble: registering the MCP server is only
// half the job. A model asked to "review this branch" reaches for gh or writes
// prose unless it is TOLD which surface to prefer — tool descriptions alone do not
// override that prior — so the preamble travels in the same {context} document
// every other injected instruction uses.
func TestSpawnChat_InjectsTheCapabilityPreamble(
	t *testing.T,
) {
	f := newFixture(t)
	f.spawn(t, "claude")

	doc := argAfter(t, f.term.calls[0].argv, "--append-system-prompt")
	assert.Contains(t, doc, "Crowbar workspace")
	assert.Contains(t, doc, "prefer them over shell equivalents")

	// The titling ASK, not just the titling tool. Retiring title_instruction took
	// the request away with the shell command, and set_chat_title then went
	// uncalled on both providers: a tool description says what a tool does, only
	// this says a title is wanted. Titling is proactive work unrelated to the
	// user's actual request, so nothing happens unless something asks.
	assert.Contains(t, doc, "set_chat_title")
}

// TestSpawnChat_Codex_InjectsTheCapabilityPreamble is codex's counterpart: it ships
// no --append-system-prompt, so the preamble rides the same silent
// developer_instructions channel a handoff uses. It must never arrive as a
// positional, which IS codex's opening user message.
//
// Together with TestResumeChat_GaplessRevive_SendsNoUserMessage below this pins the
// whole delivery rule: a FRESH spawn is injected even when it has no handoff to
// carry (the preamble is the point), a gapless RESUME is not injected at all.
func TestSpawnChat_Codex_InjectsTheCapabilityPreamble(
	t *testing.T,
) {
	f := newFixture(t)
	f.spawn(t, "codex")

	argv := f.term.calls[0].argv
	devInstructions := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, devInstructions, "Crowbar workspace")
	assert.Contains(t, devInstructions, "set_chat_title")

	for i, a := range argv {
		if i > 0 && argv[i-1] == "-c" {
			continue
		}
		assert.NotContains(t, a, "Crowbar workspace",
			"the preamble must not be injected as a codex positional arg")
	}
}

// TestComposeContext_PreambleLeadsAndAbsentHalvesLeaveNoGap pins the {context}
// document's shape at the one place it can be stated exactly.
//
// The preamble LEADS: it says which tools this CLI has and when to prefer them, and a
// model should read that before a handoff it is explicitly told not to act on. Either
// half can be missing — a user may blank capabilities_instruction in their own
// config.yaml, and a brand-new chat has no conversation — and a missing half must
// leave no stray blank line, which a model reads as a section that was meant to be
// there.
func TestComposeContext_PreambleLeadsAndAbsentHalvesLeaveNoGap(
	t *testing.T,
) {
	cases := []struct {
		name         string
		preamble     string
		conversation string
		wantDocument string
	}{{
		name:         "a fresh chat has only the preamble",
		preamble:     "PREAMBLE",
		wantDocument: "PREAMBLE",
	}, {
		name:         "a chat taking a handoff, with the preamble leading",
		preamble:     "PREAMBLE",
		conversation: "HANDOFF",
		wantDocument: "PREAMBLE\n\nHANDOFF",
	}, {
		name:         "capabilities_instruction blanked in the user's own config.yaml",
		conversation: "HANDOFF",
		wantDocument: "HANDOFF",
	}, {
		name: "nothing to say",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantDocument,
				agentusecase.ComposeContext(tc.preamble, tc.conversation))
		})
	}
}

// TestResumeChat_GaplessRevive_SendsNoUserMessage is the regression the capability
// preamble could very easily have caused. Reopening a closed tab resumes the same
// provider with nothing recorded in between, and a resumed codex can be reached
// ONLY through a user message (see codex.yaml) — so if the preamble were allowed to
// make the {context} document non-empty on its own, every reopened codex chat would
// open with a "while you were away" pointer about nothing that happened, and codex
// answers its opening message on sight.
func TestResumeChat_GaplessRevive_SendsNoUserMessage(
	t *testing.T,
) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sid-codex-native")
	turn(t, f, runnerID, "codex", "codex said something")
	require.NoError(t, f.usecase.RenameChat(f.ctx, chatID, "Already Named", "user"))
	f.wait()

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	require.Equal(t, 2, f.term.callCount())
	for i, a := range f.term.calls[1].argv {
		if i > 0 && f.term.calls[1].argv[i-1] == "-c" {
			continue
		}
		assert.NotContains(t, a, "While you were away",
			"a revive with no gap must hand the CLI no user message at all")
	}
}
