//go:build integration

// Phase 0's gate: a REAL vendor CLI must register the crowbar MCP server from the
// descriptor's injected config and successfully call a tool through the relay, the
// daemon route, the token check and the scope resolver. Nothing below this level
// proves the descriptor injection actually works against the real binaries — the
// unit tests in internal/engine/agent/descriptor_test.go only prove the argv is
// SHAPED as intended, and a well-formed flag a vendor silently ignores is
// indistinguishable from one it honours until a real binary is handed it.
//
// The two things only a real CLI can settle, and which these tests exist for:
//
//   - claude reads its whole MCP config from an inline JSON STRING
//     (--mcp-config '{...}'), and `claude mcp list` ignores the flag entirely, so
//     the flag's ACCEPTANCE cannot be observed from any non-session invocation.
//   - codex registers the server through `-c mcp_servers.crowbar.*` session
//     overrides, which are parsed as TOML rather than JSON.
//
// A title is the assertion because it is the one tool the surface has, and because
// it is state a test can read back through the same usecase the API layer does.
package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// wantTitle is the title the driven agent is asked to set, and every assertion on it
// is EXACT EQUALITY rather than containment. That is not stylistic.
//
// The chat is already titled by the time the tool could be called: the
// UserPromptSubmit hook derives a title from the prompt's first line the instant it
// is submitted (deriveTitle in internal/app/usecases/agent/agent.go), and this string
// appears verbatim inside that prompt. What makes the two distinguishable is not the
// string but the SHAPE — deriveTitle takes the whole first line, truncated at 60
// runes, so the derived title is the sentence that asks for the rename, never the two
// words being asked for. A `Contains` assertion would match that derived title and
// pass with the tool surface untouched; equality can only be satisfied by
// set_chat_title upgrading it under agent precedence.
//
// So the safeguard is the comparison operator. Keep it.
const wantTitle = "Widget Refactor"

// titlePrompt is the one turn each provider test spends. It names the tool and
// forbids the shell, and it stays that explicit even though the shell path it was
// written against no longer exists.
//
// The vaguer form ("use your crowbar tool …") was tried first and is not a weaker
// version of this one — it measures something else. Claude 2.1.220 answered it by
// running `crowbar chat rename` in a shell: config.yaml's title_instruction was in
// its context telling it to do precisely that, so the model was not ignoring an
// instruction, it was following the other one. The chat ended up correctly titled
// "Widget Refactor" WITHOUT the tool ever being called — which is the whole reason
// mcpBarrier exists, and why an assertion on the title alone would have passed on a
// tool surface that had never been reached.
//
// That finding is what retired the shell path: title_instruction and the `crowbar
// chat rename` subcommand are both gone, and set_chat_title is the only titling
// channel an agent has. The prompt stays explicit anyway, because what is under test
// here is whether a real CLI can register the server from injected config and drive a
// call through the relay, the route, the token check and the resolver — not what a
// model reaches for when left to choose. Leaving the choice in would put a
// preference measurement inside a transport test.
const titlePrompt = "Call your set_chat_title tool with the title: Widget Refactor. " +
	"Use the tool itself and run no shell command."

// mcpCall is one JSON-RPC message a vendor CLI's MCP client relayed to the daemon.
type mcpCall struct {
	Method string
	Tool   string
}

func (c mcpCall) String() string {
	if c.Tool == "" {
		return c.Method
	}
	return c.Method + "(" + c.Tool + ")"
}

// mcpBarrier observes every MCP message the daemon serves, and it is this file's
// answer to a false positive the title assertion cannot see on its own.
//
// A title on the chat is not evidence the tool ran. It never was: a shell command
// Crowbar used to ask the agent to retype landed the identical string through the
// identical RenameByRunner, and that is what this barrier was built to separate. The
// shell path is gone now, but the gap it exposed is not — deriveTitle sets a title
// from the first user prompt, a user or the FE can set one, and any future channel
// will land the same field. So "the title is Widget Refactor" proves an agent did as
// it was asked; only the relayed traffic proves the MCP server was registered,
// started and called. Which is why this exists rather than the test simply reading
// the title back.
//
// It is also a wakeup source. The tool call is NOT a hook, so hookBarrier cannot
// see it: an await keyed only on hooks would sleep through the tool call and
// re-check the title on the turn_stop hook that follows it. That is still correct,
// but it makes every pass wait out the rest of the model's turn — and it makes a
// failure look like a timeout instead of a refusal.
type mcpBarrier struct {
	sig   *kit.Signal
	mu    sync.Mutex
	calls []mcpCall
}

func newMCPBarrier() *mcpBarrier {
	return &mcpBarrier{sig: kit.NewSignal()}
}

// middleware records each MCP request on the way in and fires the barrier once its
// handler chain has fully returned — so a test woken by this edge can read whatever
// the tool wrote, with no second race to lose (the same ordering rule hookBarrier
// documents).
func (b *mcpBarrier) middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) { b.observe(ctx) }
}

func (b *mcpBarrier) observe(
	ctx *gin.Context,
) {
	if ctx.Request.Method != http.MethodPost || !strings.HasSuffix(ctx.FullPath(), "/mcp") {
		ctx.Next()
		return
	}
	b.record(snapshotBody(ctx))
	ctx.Next()
	b.sig.Fire()
}

func (b *mcpBarrier) record(
	raw []byte,
) {
	var body struct {
		RPC struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		} `json:"rpc"`
	}
	// A body too malformed to decode is still traffic, and recording it as an empty
	// call keeps it visible in a failure message rather than dropping it silently.
	_ = json.Unmarshal(raw, &body)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, mcpCall{Method: body.RPC.Method, Tool: body.RPC.Params.Name})
}

// calledTool reports whether any relayed message was a tools/call for name.
func (b *mcpBarrier) calledTool(
	name string,
) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.calls {
		if c.Method == "tools/call" && c.Tool == name {
			return true
		}
	}
	return false
}

// observed renders the MCP conversation so far for a failure message. An empty
// result means the CLI never started the server or never spoke to it — which is a
// different diagnosis from "it listed the tools and declined to call one".
func (b *mcpBarrier) observed() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.calls) == 0 {
		return "(none — the CLI never relayed a single MCP message)"
	}
	out := make([]string, 0, len(b.calls))
	for _, c := range b.calls {
		out = append(out, c.String())
	}
	return strings.Join(out, ", ")
}

// snapshotBody reads a request body and puts it back, so an observer can see what
// the handler is about to bind. Without the restore, ShouldBindJSON downstream
// reads an already-drained reader and every MCP request 400s.
func snapshotBody(
	ctx *gin.Context,
) []byte {
	raw, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return nil
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw
}

// awaitToolTitle blocks until chatID carries want, OR until the turn driven after
// priorTurns is over — whichever comes first — and leaves the verdict to the caller's
// assertions.
//
// Both arms are real signals, and the second one is what turns a refusal into a
// diagnosis. The tool call is dispatched SYNCHRONOUSLY inside the model's turn (the
// relay holds the JSON-RPC reply until the daemon has answered, and RenameByRunner
// is durable before that reply is written), so a provider that reached the tool has
// necessarily written the title BEFORE its turn_stop hook lands. The converse is
// therefore sound: a NEW assistant turn in the ledger with the title still unset means
// the agent finished and did not call the tool. Waiting past that point could only
// wait out the whole backstop for something that is never coming.
//
// priorTurns is what makes that second arm mean "the turn I drove is over" rather than
// "this chat has ever spoken", and it must be sampled by the caller BEFORE the driving
// write — a baseline taken in here would already include the turn being waited for on
// any chat with history. Pass assistantTurnCount's value from before drive();
// on a freshly spawned chat that is 0, and the arm reduces to the obvious check.
func awaitToolTitle(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	want string,
	priorTurns int,
	termSessID string,
	tap *kit.PTYTap,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	kit.Await(t, ctx, provider+" to title its chat "+want+" through the crowbar tool surface",
		func() (string, bool) {
			h.app.Repositories.WaitQuiescent()
			title := chatTitle(t, h, chatID)
			if title == want {
				return title, true
			}
			if assistantTurnCount(t, h, wsID, chatID, provider) > priorTurns {
				return title, true
			}
			// A dead CLI can satisfy neither arm, so it must be a hard failure here
			// rather than a backstop expiry five minutes later.
			requireCLIAlive(t, h, tap, termSessID, provider, "while it was being asked to title its chat")
			return title, false
		}, h.hooks.sig, h.mcp.sig)
}

// assistantTurnCount reports how many turns provider has finished in this chat, read
// off the ledger on disk. Sampled before driving, it is the baseline that lets a wait
// distinguish the turn it drove from every turn before it.
func assistantTurnCount(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
) int {
	t.Helper()
	return len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider))
}

func chatTitle(
	t *testing.T,
	h *harness,
	chatID string,
) string {
	t.Helper()
	chat, err := h.app.Usecases.Agent.GetChat(context.Background(), chatID)
	require.NoError(t, err, "read chat %s back", chatID)
	return chat.Title
}

// diagnoseOnFailure dumps the CLI's screen and the MCP conversation if the test
// fails, whatever it failed on. Registered up front rather than woven into each
// assertion because the failure that needs this most is a BACKSTOP expiry inside
// kit.Await, which has no access to either — and a vendor CLI's real complaint
// ("MCP client for `crowbar` failed to start: …", an approval prompt nobody
// answered) is painted on that screen and printed nowhere else.
func diagnoseOnFailure(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	provider string,
) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		t.Logf("MCP traffic observed: %s", h.mcp.observed())
		t.Logf("final %s screen:\n%s", provider, kit.Readable(tap.Screen()))
	})
}

// TestMCP_ClaudeTitlesItsChatThroughTheToolSurface is the claude half of the gate.
//
// It is the ONLY thing that can prove claude honours an inline --mcp-config, and
// the reason no cheaper test can: `claude mcp list` ignores the flag, and a
// deliberately malformed JSON string produces no complaint either, so "the CLI did
// not object" is not evidence. The server has to be reachable from a live session
// for the claim to mean anything.
func TestMCP_ClaudeTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-claude", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, titlePrompt)
	awaitToolTitle(t, h, wsID, chatID, "claude", wantTitle, priorTurns, termSessID, tap)

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"claude never called set_chat_title through the crowbar MCP surface. Either the inline --mcp-config "+
			"was not honoured, the relay could not reach the daemon, or the model declined the tool")
	require.Equal(t, wantTitle, chatTitle(t, h, chatID),
		"the chat's title must be the one claude set through set_chat_title")
}

// TestMCP_CodexTitlesItsChatThroughTheToolSurface is the codex half. codex parses a
// -c value as TOML and does not defer tool schemas, so its registration path shares
// nothing with claude's beyond the relay binary itself.
func TestMCP_CodexTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-codex", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	diagnoseOnFailure(t, h, tap, "codex")
	t.Logf("spawned codex: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "codex")
	drive(t, h, tap, termSessID, titlePrompt)
	awaitToolTitle(t, h, wsID, chatID, "codex", wantTitle, priorTurns, termSessID, tap)

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"codex never called set_chat_title through the crowbar MCP surface. Either the -c mcp_servers.crowbar.* "+
			"overrides did not register the server, the relay could not reach the daemon, or the model declined "+
			"the tool")
	require.Equal(t, wantTitle, chatTitle(t, h, chatID),
		"the chat's title must be the one codex set through set_chat_title")
}

// TestMCP_ForgedTokenIsRejected drives the daemon route directly rather than a CLI,
// because a real CLI has no way to send a bad token: the token it holds was minted
// by the daemon into its own argv, so the forged case is unreachable from the
// outside and would otherwise go untested.
//
// It asserts BOTH halves of failing closed. The call must be refused, and the write
// the refused call asked for must not have happened — a rejection that still
// renamed the chat would satisfy any assertion about the reply alone.
func TestMCP_ForgedTokenIsRejected(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	_, _, _, chatID, runnerID := mustSpawnChat(t, h, "claude")
	before := chatTitle(t, h, chatID)

	out, send, err := h.app.Usecases.Agent.DispatchMCP(ctx, runnerID, "forged",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Nope"}}}`))
	require.NoError(t, err)
	require.True(t, send, "a request must be answered, never met with silence")
	require.Contains(t, string(out), "unauthorized",
		"a forged token must be refused by the resolver before any tool runs")

	// The refusal rides back as a tool RESULT carrying isError (engine/mcp's
	// server.go: a tool failure is data the model reads, not a transport fault), so
	// the reply being a success envelope is expected — what must not appear in it is
	// the tool's own confirmation.
	require.NotContains(t, string(out), "Chat titled",
		"the tool must not have run at all")

	h.app.Repositories.WaitQuiescent()
	require.Equal(t, before, chatTitle(t, h, chatID),
		"a refused call must write nothing: the chat's title must be exactly what it was before")
}
