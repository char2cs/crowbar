//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EVERY MESSAGE OF A TURN, NOT JUST THE LAST ONE.
//
// The defect this file pins was reproduced by a user in one sentence: they asked
// claude to "send me a message, wait 30 seconds, then send another". claude did
// exactly that, and Crowbar's record held ONE assistant turn — the second
// message. The first was gone.
//
// The cause was that a turn's terminating hook reports a single message.
// claude's Stop carries `last_assistant_message`, which is literally the last
// one, so anything said earlier in the turn was never ingested by anything. That
// record is what get_chat_log serves to sibling agents, so the loss was not
// cosmetic: another agent read this conversation with the middle of every turn
// missing.
//
// The fix reads the provider's OWN transcript, which every hook payload already
// names and Crowbar had never opened. These tests drive the WHOLE stack over
// HTTP — the same /agent/hooks route a vendor CLI's relay posts to, and the same
// /agent/chats/:id/messages route the chat pane reads from — with a stub
// provider whose transcript this test writes itself. That is what makes the
// fallbacks testable: a real CLI cannot be asked to produce a half-written line.

// transcriptStubProviderDescriptorYAML is livestub plus a transcript
// declaration, shaped exactly like claude's: one JSON object per line,
// assistant entries carrying an array of content blocks of which only the
// `text` ones are words.
//
// It spawns `cat`, which holds its PTY open so the runner stays live across the
// whole turn.
const transcriptStubProviderDescriptorYAML = `id: transcriptstub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
    tool_pre:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
    tool_post:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
transcript:
  path_field: transcript_path
  format: jsonl
  message:
    match:
      type: assistant
      message.role: assistant
    reject:
      isSidechain: "true"
      isApiErrorMessage: "true"
    blocks: message.content
    block_match:
      type: text
    block_text: text
    join: "\n\n"
    timestamp: timestamp
`

// plainStubProviderDescriptorYAML is the SAME descriptor with the transcript
// block removed, and it exists for one assertion: a provider that declares
// nothing must behave exactly as it did before any of this was written.
const plainStubProviderDescriptorYAML = `id: plainstub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
    tool_pre:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
    tool_post:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
`

func writeProviderDescriptor(t *testing.T, h *harness, id, body string) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o644))
}

func createStubChat(t *testing.T, h *harness, imported importedRepo, provider string) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": provider}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

// postProviderHook is postAgentHook with the provider named, because these tests
// run two stub providers side by side to contrast their behaviour.
func postProviderHook(
	t *testing.T,
	h *harness,
	imported importedRepo,
	provider, segID, event, payload string,
) {
	t.Helper()
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": provider, "event": event, "payload_raw": payload,
	}, http.StatusAccepted).Body.Close()
}

type recordedMessage struct {
	Sequence int    `json:"sequence"`
	TurnID   string `json:"turnId"`
	Role     string `json:"role"`
	Text     string `json:"text"`
}

// readRecordedMessages reads a chat's conversation through the SAME route the
// chat pane reads, so what these tests assert on is what a user is shown.
func readRecordedMessages(t *testing.T, h *harness, imported importedRepo, chatID string) []recordedMessage {
	t.Helper()
	var page struct {
		Items []recordedMessage `json:"items"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID+"/messages?limit=200", &page)
	return page.Items
}

func assistantTexts(messages []recordedMessage) []string {
	out := []string{}
	for _, m := range messages {
		if m.Role == "assistant" {
			out = append(out, m.Text)
		}
	}
	return out
}

// transcriptEntry renders one claude-shaped assistant line.
func transcriptEntry(text string) string {
	line, err := json.Marshal(map[string]any{
		"type":      "assistant",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": text}},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(line) + "\n"
}

func appendTranscript(t *testing.T, path, body string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString(body)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// TestRegression_EveryAssistantMessageOfATurnIsRecorded is the user's own
// reproduction, driven through the hook route: a turn that speaks, works, and
// speaks again must land BOTH messages, in the order they were said, with the
// tool call attached to the message it followed.
//
// Before the fix the assertion below read one message where two were said. The
// second one — `last_assistant_message` — was the only thing any hook ever
// carried.
func TestRegression_EveryAssistantMessageOfATurnIsRecorded(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"session_start", `{"session_id":"sess-1","transcript_path":`+pathJSON+`}`)
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"say one, wait, say two"}`)
	h.Quiesce()

	// The agent speaks for the first time. Nothing has told Crowbar yet — no hook
	// carries this message, which is the whole defect.
	appendTranscript(t, path, transcriptEntry("MESSAGE ONE"))

	// …and then reaches for a tool, which is the hook that lets Crowbar look.
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"tool_pre", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"sleep 30"}}`)
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"tool_post", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"tool_use_id":"tool-1","tool_name":"Bash"}`)
	h.Quiesce()

	// The agent speaks again and ends the turn. THIS is the message the old code
	// recorded, and the only one.
	appendTranscript(t, path, transcriptEntry("MESSAGE TWO"))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"MESSAGE TWO"}`)
	h.Quiesce()

	messages := readRecordedMessages(t, h, imported, chatID)
	require.Equal(t, []string{"MESSAGE ONE", "MESSAGE TWO"}, assistantTexts(messages),
		"a turn that said two things must be recorded as two assistant messages, in the order they were said")

	// Order is the record's own, not a re-sort: the user's prompt comes first and
	// the two replies follow it in sequence.
	require.Len(t, messages, 3)
	assert.Equal(t, "user", messages[0].Role)
	assert.Less(t, messages[1].Sequence, messages[2].Sequence,
		"the first message the agent said must sit ABOVE the second in the record")
	assert.NotEqual(t, messages[1].TurnID, messages[2].TurnID,
		"each message needs its own turn id, or the UI cannot say which tools produced which reply")

	// And the tool call is still attached to a turn that exists — to the message
	// it followed, which is the segment it belongs to.
	var activity struct {
		ToolCalls []struct {
			ID     string `json:"id"`
			TurnID string `json:"turnId"`
		} `json:"toolCalls"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID+"/activity", &activity)
	require.Len(t, activity.ToolCalls, 1, "the tool call must be recorded exactly once")
	assert.Equal(t, messages[2].TurnID, activity.ToolCalls[0].TurnID,
		"the tool call ran in the segment that ended with the second message, and must attach to it")
}

// TestRegression_ProviderWithNoTranscriptRecordsExactlyWhatItAlwaysDid is the
// degradation contract, stated as an equality rather than a hope: the same hooks
// against a descriptor with no transcript block produce exactly the record the
// old code produced — one assistant message, the terminating hook's own.
func TestRegression_ProviderWithNoTranscriptRecordsExactlyWhatItAlwaysDid(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "plainstub", plainStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "plainstub")

	// A transcript exists on disk and is FULL of things the agent said. Nothing may
	// read it, because the descriptor does not say it can be read.
	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	appendTranscript(t, path, transcriptEntry("MESSAGE ONE")+transcriptEntry("MESSAGE TWO"))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "plainstub", runnerID,
		"session_start", `{"session_id":"sess-1","transcript_path":`+pathJSON+`}`)
	postProviderHook(t, h, imported, "plainstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	postProviderHook(t, h, imported, "plainstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"MESSAGE TWO"}`)
	h.Quiesce()

	assert.Equal(t, []string{"MESSAGE TWO"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a provider that declares no transcript must record the terminating hook's message and nothing else")
}

// TestRegression_AMalformedTranscriptStillRecordsTheReply is the property that
// matters more than the feature: a transcript that cannot be read must degrade
// to yesterday, never below it.
//
// Every hostile shape is in one file, because they all have to produce the same
// outcome — the reply, once, from the hook — and a test per shape would hide
// that they do.
func TestRegression_AMalformedTranscriptStillRecordsTheReply(t *testing.T) {
	garbage := map[string]string{
		"binary":                 "\x00\x01\x02\xff\xfe not json at all\n",
		"half written line":      `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tex`,
		"json but not an object": "[1,2,3]\n\"a bare string\"\n",
		"right shape wrong keys": `{"type":"assistant","message":{"role":"assistant","blocks":[]}}` + "\n",
		"empty":                  "",
	}
	for name, body := range garbage {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
			imported := importWritableWorkspace(t, h)
			chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

			path := filepath.Join(t.TempDir(), "session.jsonl")
			require.NoError(t, os.WriteFile(path, nil, 0o600))
			pathJSON := mustQuote(t, path)

			postProviderHook(t, h, imported, "transcriptstub", runnerID,
				"session_start", `{"session_id":"sess-1","transcript_path":`+pathJSON+`}`)
			postProviderHook(t, h, imported, "transcriptstub", runnerID,
				"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
			h.Quiesce()

			appendTranscript(t, path, body)
			postProviderHook(t, h, imported, "transcriptstub", runnerID,
				"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
					`,"last_assistant_message":"THE REPLY"}`)
			h.Quiesce()

			assert.Equal(t, []string{"THE REPLY"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
				"a transcript that cannot be read must leave the reply exactly as the hook reported it")
		})
	}
}

// TestRegression_ATranscriptThatIsGoneStillRecordsTheReply covers the case the
// shapes above cannot: no file at all, which is what a resumed session looks
// like for the instant before its CLI creates one, and what a path from a
// provider Crowbar cannot see looks like forever.
func TestRegression_ATranscriptThatIsGoneStillRecordsTheReply(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	pathJSON := mustQuote(t, filepath.Join(t.TempDir(), "never-written.jsonl"))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"THE REPLY"}`)
	h.Quiesce()

	assert.Equal(t, []string{"THE REPLY"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a transcript that does not exist must cost nothing at all")
}

// TestRegression_AResumedTranscriptsHistoryIsNeverReplayed pins the rule that
// makes reading a provider's own file safe to switch on at any moment: a session
// Crowbar has not been watching starts at the file's END.
//
// Without it, the first hook of a resumed conversation would pour that
// conversation's entire history into the record as though the agent had just
// said all of it — and every later reader, including a sibling agent asking for
// the chat log, would be handed a fabricated turn hundreds of messages long.
func TestRegression_AResumedTranscriptsHistoryIsNeverReplayed(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	appendTranscript(t, path,
		transcriptEntry("SAID LAST WEEK")+transcriptEntry("ALSO LAST WEEK")+transcriptEntry("AND AGAIN"))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"session_start", `{"session_id":"sess-1","transcript_path":`+pathJSON+`}`)
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"carry on"}`)
	h.Quiesce()

	appendTranscript(t, path, transcriptEntry("SAID JUST NOW"))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"SAID JUST NOW"}`)
	h.Quiesce()

	assert.Equal(t, []string{"SAID JUST NOW"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a resumed conversation's history is already recorded (or deliberately is not); it is never re-said")
}

// TestRegression_ReadingATranscriptWritesNothingToTheProvidersHome is the
// promise the whole feature rests on, asserted rather than documented.
//
// This is the first thing in Crowbar that opens a file the PROVIDER owns —
// something under ~/.claude or ~/.codex — and the only reason that is acceptable
// is that it exclusively reads. So the transcript here is put somewhere Crowbar
// could not write even if it tried: a read-only file in a read-only directory.
// The hooks must still work, the messages must still land, and the file must
// come back byte-for-byte identical with no sibling created beside it.
func TestRegression_ReadingATranscriptWritesNothingToTheProvidersHome(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user to mean anything")
	}
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	// A stand-in for the provider's home: Crowbar's own temp root cannot be used,
	// because the point is a directory Crowbar has no write permission on.
	home := filepath.Join(t.TempDir(), "provider-home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	path := filepath.Join(home, "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	pathJSON := mustQuote(t, path)

	// The turn opens while the file is still writable — the agent's own CLI is what
	// writes it — and only then is the whole home sealed against Crowbar.
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	h.Quiesce()
	appendTranscript(t, path, transcriptEntry("SPOKEN FIRST"))

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(home, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(home, 0o700)
		_ = os.Chmod(path, 0o600)
	})

	before, err := os.ReadFile(path) //nolint:gosec // test-owned path
	require.NoError(t, err)
	beforeInfo, err := os.Stat(path)
	require.NoError(t, err)
	beforeDir, err := os.ReadDir(home)
	require.NoError(t, err)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"SPOKEN SECOND"}`)
	h.Quiesce()

	// It really did read it — otherwise "nothing was written" would be true of a
	// code path that never ran.
	require.Equal(t, []string{"SPOKEN FIRST", "SPOKEN SECOND"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"the read-only transcript must still be READ, or this test proves nothing")

	after, err := os.ReadFile(path) //nolint:gosec // test-owned path
	require.NoError(t, err)
	afterInfo, err := os.Stat(path)
	require.NoError(t, err)
	afterDir, err := os.ReadDir(home)
	require.NoError(t, err)

	assert.Equal(t, string(before), string(after), "the transcript's bytes must be untouched")
	assert.Equal(t, beforeInfo.Size(), afterInfo.Size(), "the transcript's size must be untouched")
	assert.True(t, beforeInfo.ModTime().Equal(afterInfo.ModTime()),
		"the transcript's mtime must be untouched: opening it for writing would move it")
	assert.Equal(t, entryNames(beforeDir), entryNames(afterDir),
		"no lock file, no temp file, nothing new beside the transcript")
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestRegression_TheSameReplyIsNotRecordedTwice pins the seam between the two
// sources. A message can reach Crowbar from the file and then AGAIN from the
// terminating hook that restates it, and recording both would double every reply
// — the exact opposite failure from the one this feature fixes.
func TestRegression_TheSameReplyIsNotRecordedTwice(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	h.Quiesce()

	appendTranscript(t, path, transcriptEntry("THE ONLY REPLY"))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"THE ONLY REPLY"}`)
	h.Quiesce()

	assert.Equal(t, []string{"THE ONLY REPLY"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"one message described by two sources is one message")
}

// TestRegression_AMessageTheHookBeatsIsNotRecordedTwiceEither is the same seam
// from the other side, and it is the one that actually happens: a CLI can fire
// its terminating hook before its own transcript has been flushed. The hook's
// message is recorded, the file then hands the very same words back on the next
// read, and only the ORDER of the two sources has changed.
func TestRegression_AMessageTheHookBeatsIsNotRecordedTwiceEither(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub", transcriptStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	// The turn ends before the CLI has written the message down.
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"FLUSHED LATE"}`)
	h.Quiesce()

	// …and only then flushes it, so the next turn's first read finds it.
	appendTranscript(t, path, transcriptEntry("FLUSHED LATE"))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"again"}`)
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"THE SECOND REPLY"}`)
	h.Quiesce()

	assert.Equal(t, []string{"FLUSHED LATE", "THE SECOND REPLY"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a message the hook reported before the file did must not come back as a second copy")
}

// TestRegression_AMidTurnMessageIsTruncatedNotDropped covers the ceiling. A
// message past it is recorded MARKED rather than silently clipped, because a
// quote a reader cannot tell is partial is worse than one that says so.
func TestRegression_AMidTurnMessageIsTruncatedNotDropped(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "transcriptstub",
		transcriptStubProviderDescriptorYAML+"  max_message_bytes: 32\n")
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "transcriptstub")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	pathJSON := mustQuote(t, path)

	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"user_prompt", `{"session_id":"sess-1","transcript_path":`+pathJSON+`,"prompt":"hello"}`)
	h.Quiesce()

	appendTranscript(t, path, transcriptEntry(strings.Repeat("A", 500)))
	postProviderHook(t, h, imported, "transcriptstub", runnerID,
		"turn_stop", `{"session_id":"sess-1","transcript_path":`+pathJSON+
			`,"last_assistant_message":"and done"}`)
	h.Quiesce()

	texts := assistantTexts(readRecordedMessages(t, h, imported, chatID))
	require.Len(t, texts, 2)
	assert.LessOrEqual(t, len(texts[0]), 32+len("\n\n[crowbar: message truncated]"))
	assert.Contains(t, texts[0], "[crowbar: message truncated]",
		"a clipped message must SAY it was clipped")
	assert.Equal(t, "and done", texts[1])
}
