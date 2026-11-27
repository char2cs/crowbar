//go:build integration

package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const cwdStubProviderDescriptorYAML = `id: cwdstub
spawn:
  cmd: "sh"
  args: ["-c", "echo CROWBAR_CWD:$(pwd); exec cat"]
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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
	writeProviderDescriptor(t, h, "cwdstub", cwdStubProviderDescriptorYAML)
}

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

func readMarkedValue(
	t *testing.T,
	conn *websocket.Conn,
	marker string,
) string {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(terminalReadBound)))
	var buf strings.Builder
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "the marked value %q never arrived", marker)
		if mt != websocket.BinaryMessage {
			continue
		}
		data, _, ok := kit.ParseTerminalFrame(raw)
		if !ok {
			continue
		}
		buf.Write(data)
		if value, ok := valueAfterMarker(buf.String(), marker); ok {
			return value
		}
	}
}

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

func TestRegression_BubbleChatSpawnsInAncestorWorktree(t *testing.T) {
	h := newHarness(t)
	writeCwdStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "cwdstub", imported.workspaceID, "")
	startChat(t, h, base, ancestorID)
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)
	require.NotEmpty(t, ancestorCwd, "the ancestor's own spawn must report a real cwd")

	folder := createChatFolder(t, h, base, "bubble-holder", ancestorID)

	bubbleID := createChatWithProvider(t, h, base, "cwdstub", "", folder.ID)
	require.Empty(t, getAgentChat(t, h, base, bubbleID).WorkspaceID, "the new chat must actually be a bubble")
	startChat(t, h, base, bubbleID)

	bubbleCwd := readSpawnCwd(t, h, imported, bubbleID)
	require.Equal(t, ancestorCwd, bubbleCwd,
		"the bubble's CLI must run in its ancestor's worktree, via tree.CwdWorkspaceID's walk")
}

func TestRegression_BubbleChatReceivesAPrompt(t *testing.T) {
	h := newHarness(t)
	writeCwdStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "cwdstub", imported.workspaceID, "")
	startChat(t, h, base, ancestorID)
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)

	folder := createChatFolder(t, h, base, "prompt-bubble-holder", ancestorID)
	bubbleID := createChatWithProvider(t, h, base, "cwdstub", "", folder.ID)

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

	require.Equal(t, submission.RunnerID, getAgentChat(t, h, base, bubbleID).LiveRunnerID,
		"the dormant bubble's first prompt launched the runner that carries it")

	afterCwd := readSpawnCwd(t, h, imported, bubbleID)
	require.Equal(t, ancestorCwd, afterCwd,
		"the replacement runner promptTarget/spawnRunner placed must ALSO resolve the ancestor's worktree")
}
