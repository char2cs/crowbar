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

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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

// mcpCall is one JSON-RPC message a vendor CLI's MCP client relayed to the daemon,
// together with what the daemon answered it.
//
// Args and Reply are recorded for tools/call only, and they are what turn "the
// agent called the tool" into a DIAGNOSIS. A review tool can REFUSE a call — an
// anchor outside every changed hunk is rejected rather than stored — and the refusal
// rides back as an ordinary result carrying isError, so it is completely invisible in
// the request alone. Without the arguments there is no way to say which lines the
// model chose; without the reply there is no way to quote the sentence it was handed
// and had to recover from.
type mcpCall struct {
	Method  string
	Tool    string
	Args    string
	Reply   string
	IsError bool
}

func (c mcpCall) String() string {
	if c.Tool == "" {
		return c.Method
	}
	out := c.Method + "(" + c.Tool + " " + c.Args + ")"
	if c.IsError {
		return out + " -> REFUSED: " + c.Reply
	}
	if c.Reply == "" {
		return out
	}
	return out + " -> " + c.Reply
}

// mcpToolReply is the tool result the daemon answers a tools/call with, nested in
// the API's standard envelope: {data:{rpc:{result:{content,isError}}}}. A tool
// FAILURE is not a JSON-RPC error — engine/mcp sends it back as a successful result
// with isError set, because a model is meant to read it and try again — so the flag
// is the only thing that distinguishes a stored comment from a rejected one.
type mcpToolReply struct {
	Data struct {
		RPC struct {
			Result struct {
				Content []mcpTextContent `json:"content"`
				IsError bool             `json:"isError"`
			} `json:"result"`
		} `json:"rpc"`
	} `json:"data"`
}

type mcpTextContent struct {
	Text string `json:"text"`
}

func (r mcpToolReply) text() string {
	parts := make([]string, 0, len(r.Data.RPC.Result.Content))
	for _, c := range r.Data.RPC.Result.Content {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " ")
}

// responseTap tees a gin response body so the barrier can read what the daemon
// answered while the CLI still receives it byte for byte. gin's writer is
// write-through — the bytes are gone to the socket the moment the handler writes
// them — so an observer that only runs after ctx.Next() has nothing left to read.
type responseTap struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseTap) Write(
	p []byte,
) (int, error) {
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *responseTap) WriteString(
	s string,
) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
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
	at := b.record(snapshotBody(ctx))
	tap := &responseTap{ResponseWriter: ctx.Writer, body: &bytes.Buffer{}}
	ctx.Writer = tap
	ctx.Next()
	b.recordReply(at, tap.body.Bytes())
	b.sig.Fire()
}

// record appends the request and returns where it landed, so the reply that comes
// back after ctx.Next() can be attached to the call it answers rather than to
// whatever a concurrently served runner happened to send in the meantime.
func (b *mcpBarrier) record(
	raw []byte,
) int {
	var body struct {
		RPC struct {
			Method string `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		} `json:"rpc"`
	}
	// A body too malformed to decode is still traffic, and recording it as an empty
	// call keeps it visible in a failure message rather than dropping it silently.
	_ = json.Unmarshal(raw, &body)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, mcpCall{
		Method: body.RPC.Method,
		Tool:   body.RPC.Params.Name,
		Args:   string(body.RPC.Params.Arguments),
	})
	return len(b.calls) - 1
}

// recordReply attaches the daemon's answer to the call recorded at index at.
//
// Only a tools/call reply is kept. An initialize or tools/list body is the entire
// schema catalogue of every crowbar tool, and dumping that into a failure message
// would bury the one line that explains the failure.
func (b *mcpBarrier) recordReply(
	at int,
	raw []byte,
) {
	var reply mcpToolReply
	// A notification is answered with a bare 204 and no body at all, so a decode
	// failure here is ordinary rather than exceptional: there is simply no result to
	// attach.
	if json.Unmarshal(raw, &reply) != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if at < 0 || at >= len(b.calls) || b.calls[at].Method != "tools/call" {
		return
	}
	b.calls[at].Reply = reply.text()
	b.calls[at].IsError = reply.Data.RPC.Result.IsError
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
	awaitToolEffect(
		t, h, wsID, chatID, provider, termSessID, tap, priorTurns,
		provider+" to title its chat "+want+" through the crowbar tool surface",
		"while it was being asked to title its chat",
		func() bool { return chatTitle(t, h, chatID) == want },
	)
}

// awaitToolEffect is awaitToolTitle's rule generalised to any tool: it blocks until
// effect holds — the thing a tool was supposed to DO having happened — or until the
// turn driven after priorTurns is over, whichever comes first, and leaves the verdict
// to the caller's assertions.
//
// The two-armed shape is what turns a refusal into a diagnosis rather than a timeout,
// and it is sound for every tool on this surface for the same reason it is sound for
// a title: the call is dispatched SYNCHRONOUSLY inside the model's turn — the relay
// holds the JSON-RPC reply until the daemon has answered, and every write is durable
// before that reply is written — so an agent that reached the tool has necessarily
// produced the effect BEFORE its turn_stop hook lands. A new assistant turn with the
// effect still absent therefore means the agent finished and did not call the tool
// (or called it and was refused), and waiting past that point could only wait out the
// whole backstop for something that is never coming.
//
// priorTurns must be sampled by the CALLER before the driving write; see
// awaitToolTitle's comment for why a baseline taken in here would be useless.
func awaitToolEffect(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	termSessID string,
	tap *kit.PTYTap,
	priorTurns int,
	what string,
	when string,
	effect func() bool,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	kit.Await(t, ctx, what, func() (bool, bool) {
		h.app.Repositories.WaitQuiescent()
		if effect() {
			return true, true
		}
		if assistantTurnCount(t, h, wsID, chatID, provider) > priorTurns {
			return false, true
		}
		// A dead CLI can satisfy neither arm, so it must be a hard failure here
		// rather than a backstop expiry five minutes later.
		requireCLIAlive(t, h, tap, termSessID, provider, when)
		return false, false
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
	chat, err := h.app.Usecases.AgentChat.GetChat(context.Background(), chatID)
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

	out, send, err := h.app.Usecases.AgentProvider.DispatchMCP(ctx, runnerID, "forged",
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

// ---------------------------------------------------------------------------
// Phase 1's gate: a real model answering and leaving real review threads.
//
// Everything above this line is about the TRANSPORT — that a vendor CLI can be
// handed a server it will register and reach. The two tests below are about the
// PRODUCT: given a real diff and a real review comment, a real claude must answer
// that comment as a thread reply and report its own findings as anchored threads,
// rather than as prose in a chat nobody is reviewing in.
//
// Neither prompt names a tool, and that is the point. titlePrompt above deliberately
// does ("call your set_chat_title tool") because what it measures is registration, and
// leaving the choice in would have put a preference measurement inside a transport
// test. Here the choice IS the measurement: the capability preamble
// (config/default.yaml's capabilities_instruction) tells the model that code review
// happens in Crowbar and to post findings as anchored threads, and these two tests are
// the only thing that can say whether a real model actually obeys that over the shell
// and chat-prose habits it already has.
// ---------------------------------------------------------------------------

// reviewFile is the one file the review fixture puts in the branch diff.
const reviewFile = "billing.js"

// reviewFileBase is the file as the BASE branch has it: correct, and long enough that
// the branch's one-line change produces a hunk covering only part of it.
//
// That length is load-bearing. post_review_comment validates an anchor against the
// review's hunk geometry, so a fixture whose whole file is one hunk would accept any
// line the model named and prove nothing about whether a real model can pick an anchor
// the surface accepts. Here the hunk is `@@ -31,7 +31,7 @@` — right-side lines 31..37
// — so an anchor on the defect is accepted and an anchor at, say, the top of the file
// is refused.
const reviewFileBase = `// Order helpers used by the billing report.

function orderTotal(order) {
  let total = 0;
  for (const line of order.lines) {
    total += line.quantity * line.unitPrice;
  }
  return total;
}

function formatAmount(cents) {
  return "$" + (cents / 100).toFixed(2);
}

function reportLines(orders) {
  const out = [];
  for (const order of orders) {
    out.push({
      id: order.id,
      total: formatAmount(orderTotal(order)),
    });
  }
  return out;
}

function averageOrderValue(orders) {
  if (orders.length === 0) {
    return 0;
  }
  let total = 0;
  for (const order of orders) {
    total += orderTotal(order);
  }
  return total / orders.length;
}

module.exports = { orderTotal, formatAmount, reportLines, averageOrderValue };
`

// correctDivisor and buggyDivisor are the branch's entire change: an average divided
// by one fewer than the number of things averaged.
//
// The defect has to be unmistakable, because the second test asserts that the agent
// posts A FINDING and a model asked to review clean code has nothing honest to report
// — a refusal to invent one would look identical to a refusal to use the tool. This
// one is wrong arithmetically for every input and divides by zero for a single order,
// so any reviewer flags it, and it is one line so the hunk stays small.
const (
	correctDivisor = "  return total / orders.length;"
	buggyDivisor   = "  return total / (orders.length - 1);"
)

// userThreadBody is the review comment the first test asks the agent to answer. It is
// a QUESTION, so the only way to address it is to say something back — an agent that
// merely fixed the code would leave the thread unanswered.
const userThreadBody = "Why does this divide by `orders.length - 1`? With exactly one order " +
	"that divides by zero. Was it deliberate?"

// reviewReplyPrompt and reviewFindingPrompt name no tool and no mechanism. See the
// banner above: whether the model reaches for the review surface unprompted is the
// thing under test.
const (
	reviewReplyPrompt   = "There is a review comment on this branch. Read it and reply to it."
	reviewFindingPrompt = "Review this branch and post any finding as a review comment."
)

// reviewFixture builds the workspace both Phase 1 tests review: a managed child
// worktree whose branch carries one committed, plainly defective change against its
// base. It returns the workspace and the right-side line the defect sits on.
//
// It ASSERTS the fixture before any model call is spent on it. A fixture that silently
// stopped producing a diff — a base file edited out from under buggyVariant, a
// CreateChild that forked from the wrong ref — would leave the agent nothing to review
// and nothing it could legally anchor to, and the test would then report a model that
// "declined to post a finding" when in truth there was none to make. Proving the diff
// and its geometry up front means a failure downstream is a statement about the agent.
func reviewFixture(
	t *testing.T,
	h *harness,
	name string,
) (string, int) {
	t.Helper()

	repoPath := kit.InitRepoWithFile(t, reviewFile, reviewFileBase)
	_, _, wsID := h.importRepoAndWorkspace(t, name, repoPath)

	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err, "resolve the fixture workspace's worktree")
	branchContent := buggyVariant(t)
	kit.CommitFile(t, ws.WorktreePath, reviewFile, branchContent, "average orders over the wrong divisor")

	require.Contains(t, changedFiles(t, h, wsID), reviewFile,
		"the fixture's committed change must appear in the branch review, or the agent has nothing to review")
	line := defectLine(t, branchContent)
	requireAnchorable(t, h, wsID, reviewFile, line)
	return wsID, line
}

// buggyVariant returns the base file with the fixture's one deliberate defect
// introduced, and fails if the line it rewrites is not there: a silent no-op would
// leave the branch identical to its base.
func buggyVariant(
	t *testing.T,
) string {
	t.Helper()
	out := strings.Replace(reviewFileBase, correctDivisor, buggyDivisor, 1)
	require.NotEqual(t, reviewFileBase, out,
		"the fixture's base file no longer contains %q, so the branch would carry no change at all", correctDivisor)
	return out
}

// defectLine reports the 1-based line the defect sits on, computed from the content
// rather than written down — the anchor the fixture proves valid and the anchor the
// user's thread is left on must both follow the file if it is ever edited.
func defectLine(
	t *testing.T,
	content string,
) int {
	t.Helper()
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, buggyDivisor) {
			return i + 1
		}
	}
	t.Fatalf("the fixture's branch content does not contain %q", buggyDivisor)
	return 0
}

// requireAnchorable proves that line is inside a right-side hunk of the workspace's
// branch diff, so post_review_comment would accept an anchor there. It is the fixture
// half of the anchor contract: with it established, a refused anchor can only be a line
// the MODEL chose, which is a finding about the tool's usability rather than about the
// fixture.
func requireAnchorable(
	t *testing.T,
	h *harness,
	wsID string,
	path string,
	line int,
) {
	t.Helper()
	outline, err := h.app.Usecases.BranchReview.GetOutline(context.Background(), wsID, "")
	require.NoError(t, err, "read the review's hunk geometry for workspace %s", wsID)
	for _, f := range outline {
		if f.Path == path && hunkCovers(f.Hunks, line) {
			t.Logf("fixture geometry: %s hunks=%+v, defect on right-side line %d", path, f.Hunks, line)
			return
		}
	}
	t.Fatalf("line %d of %s is in no changed hunk of this review, so no anchored comment could ever be "+
		"posted there; outline observed: %+v", line, path, outline)
}

func hunkCovers(
	hunks []gitdomain.HunkShape,
	line int,
) bool {
	for _, h := range hunks {
		if line >= h.NewStart && line <= h.NewStart+h.NewLines-1 {
			return true
		}
	}
	return false
}

// openUserThread leaves the review comment the agent is asked to answer, through the
// SAME usecase the review pane's thread endpoint calls — so what the agent finds is
// shaped exactly like a user's comment, down to the empty author a human message
// carries (BranchReview.OpenThread records none).
func openUserThread(
	t *testing.T,
	h *harness,
	wsID string,
	line int,
) string {
	t.Helper()
	thread, err := h.app.Usecases.BranchReview.OpenThread(context.Background(), branchreview.OpenThreadInput{
		WsID:       wsID,
		FilePath:   reviewFile,
		LineNumber: line,
		Side:       domain.ReviewSideRight,
		Body:       userThreadBody,
	})
	require.NoError(t, err, "open the user's review thread")
	return thread.ID
}

// readThread reads a thread back by id. It goes through the repository's Get, which
// folds the aggregate from the event log rather than reading the projected read model,
// so it is current the instant the tool's write returned.
func readThread(
	t *testing.T,
	h *harness,
	threadID string,
) domain.ReviewThread {
	t.Helper()
	thread, err := h.app.Repositories.ReviewThread.Get(context.Background(), threadID)
	require.NoError(t, err, "read review thread %s back", threadID)
	return thread
}

// agentThreads lists the workspace's review threads an AGENT opened — the ones whose
// first message is the agent's own finding, as opposed to a user comment an agent later
// replied to.
func agentThreads(
	t *testing.T,
	h *harness,
	wsID string,
) []domain.ReviewThread {
	t.Helper()
	threads, err := h.app.Repositories.ReviewThread.ListByWorkspace(context.Background(), wsID)
	require.NoError(t, err, "list the review threads on workspace %s", wsID)
	out := make([]domain.ReviewThread, 0, len(threads))
	for _, thread := range threads {
		if len(thread.Messages) > 0 && thread.Messages[0].IsAgent {
			out = append(out, thread)
		}
	}
	return out
}

// changedFiles is the review's changed-file list, read through the same usecase
// get_review_scope reports to the agent.
func changedFiles(
	t *testing.T,
	h *harness,
	wsID string,
) []string {
	t.Helper()
	files, err := h.app.Usecases.BranchReview.GetFiles(context.Background(), wsID, "")
	require.NoError(t, err, "read the review's changed files for workspace %s", wsID)
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// TestMCP_ClaudeAnswersAReviewThread is the product thesis end to end: a real claude,
// given a real diff and a real review comment, must answer that comment where the
// comment lives.
//
// The assertion is deliberately in three parts, and the first one is the one that
// matters. A thread with two messages says an agent replied; only h.mcp's record of the
// relayed tools/call says it replied THROUGH THE TOOL. Task 9 learned that the hard
// way — claude titled a chat correctly by running a shell command, so a state-only
// assertion passed on a tool surface that had never been reached — and a reply is
// exactly as forgeable: nothing stops a model from editing the review database's own
// files, or from claiming in chat that it answered.
func TestMCP_ClaudeAnswersAReviewThread(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, line := reviewFixture(t, h, "mcp-review-reply")
	threadID := openUserThread(t, h, wsID, line)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s thread=%s home=%s",
		chatID, runnerID, wsID, threadID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, reviewReplyPrompt)
	awaitToolEffect(
		t, h, wsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to answer review thread "+threadID+" through the crowbar tool surface",
		"while it was being asked to answer a review comment",
		func() bool { return len(readThread(t, h, threadID).Messages) > 1 },
	)

	require.True(t, h.mcp.calledTool("reply_to_review_thread"),
		"claude never called reply_to_review_thread. It either answered in chat prose instead of in the review, "+
			"or reached the thread by some other means — MCP traffic observed: %s", h.mcp.observed())
	thread := readThread(t, h, threadID)
	require.Len(t, thread.Messages, 2,
		"the user's thread must now carry exactly two messages: the comment and the agent's answer")
	require.True(t, thread.Messages[1].IsAgent,
		"the answer must be recorded as the AGENT's, or the review UI attributes it to the user")

	t.Logf("MCP traffic observed: %s", h.mcp.observed())
	t.Logf("agent's answer: %s", thread.Messages[1].Body)
}

// TestMCP_ClaudePostsAFindingAsAnAnchoredThread is the other half of the thesis: an
// agent's OWN review findings must land as anchored threads in Crowbar's review, not as
// prose in the chat.
//
// It is also the only test that exercises the anchor contract against a real model.
// The fixture has already proved (requireAnchorable) that the defect's line is inside a
// changed hunk, so every anchor the surface refuses here is a line the model chose for
// itself — and the refusal text it received is recorded on the MCP traffic log, because
// whether a real model can satisfy these rules from get_review_scope alone is precisely
// what Phase 1 is meant to find out.
func TestMCP_ClaudePostsAFindingAsAnAnchoredThread(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, _ := reviewFixture(t, h, "mcp-review-post")

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, reviewFindingPrompt)
	awaitToolEffect(
		t, h, wsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to post a finding as an anchored review thread",
		"while it was being asked to review the branch",
		func() bool { return len(agentThreads(t, h, wsID)) > 0 },
	)

	require.True(t, h.mcp.calledTool("post_review_comment"),
		"claude never called post_review_comment. It either wrote its findings as chat prose, or reviewed "+
			"nothing at all — MCP traffic observed: %s", h.mcp.observed())
	threads := agentThreads(t, h, wsID)
	require.NotEmpty(t, threads,
		"post_review_comment was called but no agent thread exists on the workspace, so every call was "+
			"REFUSED — MCP traffic observed: %s", h.mcp.observed())

	// Every stored thread, not merely one: an anchor on a file outside the review is
	// refused before the store is touched, so this also asserts that the validation
	// which produced these threads did its job.
	changed := changedFiles(t, h, wsID)
	for _, thread := range threads {
		require.Contains(t, changed, thread.FilePath,
			"the agent anchored a finding to %s, which is not in this review's changed files %v",
			thread.FilePath, changed)
		t.Logf("agent finding: %s:%d-%d (%s side): %s",
			thread.FilePath, thread.StartLine, thread.EndLine, thread.Side, thread.Messages[0].Body)
	}

	t.Logf("MCP traffic observed: %s", h.mcp.observed())
}

// taskPrompt is a plain unit of work. It says nothing about titles, nothing about
// tools, and nothing about Crowbar — which is the entire point.
//
// The two titling tests above measure TRANSPORT: they name the tool, so they prove a
// real CLI can register the server from injected config and drive a call through the
// relay, the route, the token check and the resolver. They deliberately leave no
// choice to the model. This measures the choice.
//
// It exists because retiring title_instruction removed the shell command AND the
// request to title along with it. set_chat_title stayed registered, but a tool
// description states a capability, not an expectation — so neither provider titled a
// chat at all, and every test still passed, because every test told the model to
// title. The preamble now asks. This is what checks the asking works.
const taskPrompt = "Read README.md and tell me in one sentence what this repository is."

// spontaneousTitling drives one task-shaped turn and asserts the agent titled the
// chat without being asked to.
//
// The assertion is on the MCP traffic, not on the title text. A chat is ALREADY
// titled by the time the tool could be called — the UserPromptSubmit hook derives one
// from the prompt's first line — so a non-empty title proves nothing. Only a
// tools/call for set_chat_title distinguishes "the agent chose to title this" from
// "the daemon derived a title on its own".
func spontaneousTitling(
	t *testing.T,
	provider string,
) {
	t.Helper()
	requireCLI(t, provider)
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-spontaneous-"+provider, repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, provider)
	diagnoseOnFailure(t, h, tap, provider)
	t.Logf("spawned %s: chat=%s runner=%s workspace=%s", provider, chatID, runnerID, wsID)

	derived := chatTitle(t, h, chatID)
	priorTurns := assistantTurnCount(t, h, wsID, chatID, provider)
	drive(t, h, tap, termSessID, taskPrompt)

	awaitToolEffect(t, h, wsID, chatID, provider, termSessID, tap, priorTurns,
		provider+" to title its own chat unprompted",
		"while it worked on a task that never mentioned titling",
		func() bool { return h.mcp.calledTool("set_chat_title") })

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"%s finished a turn without titling its chat. The capability preamble asks for a title and "+
			"set_chat_title is registered, so either the ask is not reaching the model or it declined "+
			"it — MCP traffic observed: %s", provider, h.mcp.observed())

	got := chatTitle(t, h, chatID)
	require.NotEqual(t, derived, got,
		"set_chat_title was called but the title is unchanged from the one the daemon derived at "+
			"prompt submission, so the call did not take effect")
	t.Logf("%s titled its chat unprompted: %q (was %q). MCP traffic: %s",
		provider, got, derived, h.mcp.observed())
}

func TestMCP_ClaudeTitlesItsChatWithoutBeingAsked(t *testing.T) {
	spontaneousTitling(t, "claude")
}

func TestMCP_CodexTitlesItsChatWithoutBeingAsked(t *testing.T) {
	spontaneousTitling(t, "codex")
}
