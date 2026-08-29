//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// cwdStubProviderDescriptorYAML spawns a shell that prints its OWN real OS
// working directory, prefixed by a marker, and then execs into `cat` so the
// PTY (and the runner) stays alive exactly like livestub's — see
// writeLiveStubProviderDescriptor's own doc. The printed line is read back
// off the runner's real terminal WS by readSpawnCwd: ground truth no
// Crowbar-reported field can fake, since it is text the spawned OS process
// itself produced.
//
// It also declares presentation.prompt_submit (restart_tui, {message} passed
// positionally to sh — a harmless extra $0 the script never reads) so
// SubmitPrompt's replacement spawn is reachable: that path resolves its own
// cwd through promptTarget, a SEPARATE call site from the create path's
// spawnPaths, and is exactly what TestRegression_BubbleChatReceivesAPrompt
// exercises. session.resume is declared only because prompt_submit's own
// validation requires it present; this test never resumes, so it is never
// actually applied to an argv.
const cwdStubProviderDescriptorYAML = `id: cwdstub
spawn:
  cmd: "sh"
  args: ["-c", "echo CROWBAR_CWD:$(pwd); exec cat"]
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func writeCwdStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cwdstub.yaml"), []byte(cwdStubProviderDescriptorYAML), 0o644))
}

// createChatWithProvider creates a chat with an explicit provider, optional
// workspaceID and parentID, and quiesces past the async runner placement
// (see createAgentChat's own doc for why QuiesceReactors, not Quiesce, is the
// correct barrier here).
func createChatWithProvider(
	t *testing.T,
	h *harness,
	base string,
	provider string,
	workspaceID string,
	parentID string,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats",
		map[string]string{"provider": provider, "workspaceId": workspaceID, "parentId": parentID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.QuiesceReactors()
	return created.ID
}

// readSpawnCwd dials chatID's runner PTY and reads back the real OS working
// directory the cwdstub shell reported at spawn.
//
// It reads off the TERMINAL WS (readTerminalUntil's own primitive, see
// terminal_test.go), not any Crowbar-side field, because there is no such
// field on the wire, and because the process's own stdout is the one signal
// nothing internal to Crowbar can fake. A newly attaching client always
// receives the terminal's current screen redraw before any live output
// (session.Session.Attach's serialize-on-attach contract), so the marker is
// found whether the echo happened before or after this dial.
//
// The terminal WS route is workspace-scoped (.../workspaces/:wsId/terminals/
// :sessionId/ws), but WS itself only checks the session id exists — it never
// checks the session's OWN registered workspace against :wsId (see
// terminal/handlers/ws.go) — so wsID names any REAL workspace the caller can
// reach, imported.workspaceID for both the ancestor's own session and the
// bubble's (which carries none of its own).
func readSpawnCwd(
	t *testing.T,
	h *harness,
	imported importedRepo,
	chatID string,
) string {
	t.Helper()
	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.NotEmpty(t, detail.LiveRunnerID, "a live chat must carry a live runner")
	require.NotEmpty(t, detail.TerminalSessionID, "a live chat must carry a terminal session")

	conn := h.dial(wsBase(imported) + "/terminals/" + detail.TerminalSessionID + "/ws")
	return readMarkedValue(t, conn, "CROWBAR_CWD:")
}

// readMarkedValue blocks reading PTY frames, accumulating their data, until
// marker appears followed by a run of non-control bytes terminated by a
// control byte (a redraw's cursor move, or a raw \r/\n) — the value the
// process printed right after the marker, trimmed of any row padding a full
// grid redraw adds.
//
// It carries no read deadline, like readTerminalUntil: the PTY output IS the
// signal, and a value that never arrives is a hang `go test -timeout` reports
// against this exact read, not a flaky timeout.
func readMarkedValue(
	t *testing.T,
	conn *websocket.Conn,
	marker string,
) string {
	t.Helper()
	var buf strings.Builder
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "PTY ws closed before the marked value arrived")
		if mt != websocket.TextMessage {
			continue
		}
		var msg struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		buf.WriteString(msg.Data)
		if value, ok := valueAfterMarker(buf.String(), marker); ok {
			return value
		}
	}
}

// valueAfterMarker extracts the run of bytes following marker in text, up to
// (not including) the first control byte, and reports false when text ends
// before any control byte closes that run — meaning the value may still be
// arriving in a later frame.
func valueAfterMarker(
	text string,
	marker string,
) (string, bool) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	rest := text[idx+len(marker):]
	for i := 0; i < len(rest); i++ {
		if rest[i] < 0x20 || rest[i] == 0x7f {
			return strings.TrimSpace(rest[:i]), true
		}
	}
	return "", false
}

// TestRegression_BubbleChatSpawnsInAncestorWorktree proves the fix wiring
// tree.CwdWorkspaceID's ancestor walk into the spawn path (Task 22): a bubble
// (Chat.WorkspaceID == "") nested under a worktree-owning chat's own folder
// subtree — the construction regression_sidebar_forest_test.go's own
// disclosed departure #1 settled for, since a chat parent enforces
// ErrCrossWorkspace unless the immediate parent is a folder — spawns
// successfully and its CLI's REAL OS-level working directory is the
// ancestor's worktree, not empty or a 500.
//
// Before this fix this reproduced the exact 500
// ("spawn runner: worktree dir: ... aggregate not found") Task 21's own
// investigation found live: StartRunner resolved the runner's cwd from
// chat.WorkspaceID verbatim, with no fallback to the ancestor walk.
func TestRegression_BubbleChatSpawnsInAncestorWorktree(t *testing.T) {
	h := newHarness(t)
	writeCwdStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "cwdstub", imported.workspaceID, "")
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)
	require.NotEmpty(t, ancestorCwd, "the ancestor's own spawn must report a real cwd")

	folder := createChatFolder(t, h, base, "bubble-holder", ancestorID)

	bubbleID := createChatWithProvider(t, h, base, "cwdstub", "", folder.ID)
	detail := getAgentChat(t, h, base, bubbleID)
	require.Empty(t, detail.WorkspaceID, "the new chat must actually be a bubble")
	require.NotEmpty(t, detail.LiveRunnerID,
		"a bubble nested under a worktree-owning chat's folder must spawn a live runner, not 500")

	bubbleCwd := readSpawnCwd(t, h, imported, bubbleID)
	require.Equal(t, ancestorCwd, bubbleCwd,
		"the bubble's CLI must run in its ancestor's worktree, via tree.CwdWorkspaceID's walk")
}

// TestRegression_BubbleChatReceivesAPrompt proves the SAME ancestor-cwd
// fallback also covers a bubble's very first follow-up message, not merely
// its create.
//
// This is a genuinely SEPARATE call site from the one
// TestRegression_BubbleChatSpawnsInAncestorWorktree exercises:
// SubmitPrompt's own preflight (promptTarget, prompts.go) used to resolve
// WorktreeDir from chat.WorkspaceID directly, so a bubble could be CREATED
// successfully and still 500/404 on its very next message. Compact and
// SlashCatalog resolve cwd through the identical helper
// (cwdWorkspaceID) and are covered at the usecase level instead — see
// TestCompact_ResolvesCwdThroughTheAncestorWalkForABubble and
// TestSlashCatalog_ResolvesCwdThroughTheAncestorWalkForABubble in
// internal/app/usecases/chat, both reached over the real HTTP surface would
// need a genuine terminal-attached provider to answer either capability
// check meaningfully, which adds nothing this end-to-end test and those two
// unit tests don't already prove together.
func TestRegression_BubbleChatReceivesAPrompt(t *testing.T) {
	h := newHarness(t)
	writeCwdStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "cwdstub", imported.workspaceID, "")
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)

	folder := createChatFolder(t, h, base, "prompt-bubble-holder", ancestorID)
	bubbleID := createChatWithProvider(t, h, base, "cwdstub", "", folder.ID)
	before := getAgentChat(t, h, base, bubbleID)
	require.NotEmpty(t, before.LiveRunnerID, "the bubble must be live before it can take a prompt")

	var submission struct {
		RunnerID          string `json:"runnerId"`
		TerminalSessionID string `json:"terminalSessionId"`
	}
	h.post(base+"/chats/"+bubbleID+"/prompts",
		map[string]string{"text": "hello from the test", "clientRequestId": uuid.NewString()},
		http.StatusOK, &submission)
	require.NotEmpty(t, submission.RunnerID,
		"submitting a prompt to a bubble must place a runner to carry it, not 500/404 in promptTarget")
	h.QuiesceReactors()

	after := getAgentChat(t, h, base, bubbleID)
	require.NotEmpty(t, after.LiveRunnerID, "the bubble must still be live after taking the prompt")
	require.NotEqual(t, before.LiveRunnerID, after.LiveRunnerID,
		"restart_tui delivery replaces the CLI, so a NEW runner is the one that actually carries the prompt")

	afterCwd := readSpawnCwd(t, h, imported, bubbleID)
	require.Equal(t, ancestorCwd, afterCwd,
		"the replacement runner promptTarget/spawnRunner placed must ALSO resolve the ancestor's worktree")
}
