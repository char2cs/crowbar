package agents_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

const typedPromptDescriptor = `
id: typist
spawn:
  cmd: typist-cli
  interactive_required: true
events:
  session_start:
    in: session_start
    map: { session_id: session_id }
  turn_stop:
    in: turn_stop
    map: { message: last }
session:
  resume: { arg: "--resume {id}" }
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
    steer:
      skip_prefixes: ["/"]
      frame: "from the user: {message}"
      reply: '{"decision":"block","reason":{message_json}}'
    type:
      paste: bracketed
      submit: "\r"
      skip_prefixes: ["!", "/btw"]
      composer:
        row_prefix: "❯ "
`

func typist(t *testing.T) agents.Agent {
	t.Helper()
	home := t.TempDir()
	writeDescriptor(t, home, "typist", typedPromptDescriptor)
	a, err := agents.New().Get(context.Background(), home, "typist")
	require.NoError(t, err)
	return a
}

func TestAgent_PromptTypeReportsWhatTheDescriptorDeclares(t *testing.T) {
	typing, ok := typist(t).PromptType()
	require.True(t, ok)
	assert.Equal(t, agents.PromptTyping{
		Paste: "bracketed", Submit: "\r", ComposerRowPrefix: "❯ ", SkipPrefixes: []string{"!", "/btw"},
	}, typing, "the row prefix keeps its no-break space")

	_, ok = get(t, "codex").PromptType()
	assert.False(t, ok, "a provider that declares no type block is never typed into")
}

func TestAgent_ClaudeDeclaresTypedDeliveryWithItsNoBreakSpacePrefix(t *testing.T) {
	typing, ok := get(t, "claude").PromptType()

	require.True(t, ok)
	assert.Equal(t, agents.PromptTyping{
		Paste: "bracketed", Submit: "\r", ComposerRowPrefix: "❯ ", SkipPrefixes: []string{"/", "!"},
	}, typing)
	assert.Equal(t, []byte{0xe2, 0x9d, 0xaf, 0xc2, 0xa0}, []byte(typing.ComposerRowPrefix))
}

func TestAgent_PromptTypeBytesPastesTheMessageThenSubmits(t *testing.T) {
	a := typist(t)

	got, ok := a.PromptTypeBytes("first line\nsecond \"line\"\r\n\x1b[31m{message}")
	require.True(t, ok)
	assert.Equal(t, "\x1b[200~first line\nsecond \"line\"\r\n\x1b[31m{message}\x1b[201~\r", string(got))

	got, ok = a.PromptTypeBytes("")
	require.True(t, ok)
	assert.Equal(t, "\x1b[200~\x1b[201~\r", string(got))
}

func TestAgent_PromptTypeBytesRefusesWhatWouldEndThePasteEarlyOrIsACommand(t *testing.T) {
	a := typist(t)

	for name, text := range map[string]string{
		"end-paste sequence mid-message": "hi\x1b[201~\rrm -rf /",
		"end-paste sequence alone":       "\x1b[201~",
		"a message the type block skips": "!ls",
		"a longer skipped prefix":        "/btw and so on",
	} {
		got, ok := a.PromptTypeBytes(text)
		assert.False(t, ok, name)
		assert.Empty(t, got, name)
	}

	got, ok := a.PromptTypeBytes("/compact")
	require.True(t, ok, "only the type block's own skip list applies, not steer's")
	assert.Equal(t, "\x1b[200~/compact\x1b[201~\r", string(got))

	_, ok = get(t, "codex").PromptTypeBytes("hello")
	assert.False(t, ok, "a provider that declares no type block renders nothing")
}

func TestAgent_CanTypeIsFalseForSkippedOrUnsafeMessages(t *testing.T) {
	a := typist(t)
	assert.True(t, a.CanType("hello\nworld"))
	assert.True(t, a.CanType("not a !command"))
	assert.False(t, a.CanType("!ls"))
	assert.False(t, a.CanType("/btw hi"))
	assert.False(t, a.CanType("x\x1b[201~"))
	assert.False(t, get(t, "codex").CanType("hello"))
}

// composerRecording is a screen recorded from the real CLI through our emulator
// plus the cursor it left, as the terminal read reports them.
func composerRecording(t *testing.T, state string) (screen string, row, col int) {
	t.Helper()
	text, err := os.ReadFile("testdata/claude-composer-" + state + "-2.1.325.screen")
	require.NoError(t, err)
	cursor, err := os.ReadFile("testdata/claude-composer-" + state + "-2.1.325.cursor")
	require.NoError(t, err)
	var paste bool
	_, err = fmt.Sscanf(string(cursor), "%d %d %t", &row, &col, &paste)
	require.NoError(t, err)
	return string(text), row, col
}

// typistWithPrefix is the typist descriptor with its composer row prefix swapped.
func typistWithPrefix(t *testing.T, prefix string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	body := strings.Replace(typedPromptDescriptor, `row_prefix: "❯ "`, "row_prefix: "+strconv.Quote(prefix), 1)
	require.NotEqual(t, typedPromptDescriptor, body)
	writeDescriptor(t, home, "typist", body)
	a, err := agents.New().Get(context.Background(), home, "typist")
	require.NoError(t, err)
	return a
}

func TestComposerEmpty_ClaudeReadsEachRecordedScreen(t *testing.T) {
	cases := []struct {
		state string
		empty bool
	}{
		{"idle-fresh-80x24", true},
		{"idle-fresh-120x40", true},
		{"idle-ghost-80x24", true},
		{"idle-ghost-120x40", true},
		// The composer reads empty mid-turn too: the ledger is what guards it.
		{"midturn-80x24", true},
		{"draft-80x24", false},
		{"draft-120x40", false},
		{"help-dialog-80x24", false},
		{"paste-bang-80x24", false},
		{"paste-escaped-bang-80x24", false},
		{"paste-multiline-80x24", false},
		{"slash-typed-80x24", false},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			screen, row, col := composerRecording(t, tc.state)

			assert.Equal(t, tc.empty, get(t, "claude").ComposerEmpty(screen, row, col))
		})
	}
}

func TestComposerEmpty_APlainSpacePrefixNeverMatchesTheNoBreakSpaceRecording(t *testing.T) {
	screen, row, col := composerRecording(t, "idle-fresh-80x24")

	assert.False(t, typistWithPrefix(t, "❯ ").ComposerEmpty(screen, row, col))
	assert.True(t, typistWithPrefix(t, "❯ ").ComposerEmpty(screen, row, col))
}

func TestComposerEmpty_JudgesTheCursorRowAgainstThePrefixDisplayWidth(t *testing.T) {
	a := typistWithPrefix(t, "あ> ")
	screen := "banner\nあ> \nfooter"

	assert.True(t, a.ComposerEmpty(screen, 1, 4), "a wide glyph takes two cells: 2 + '>' + ' '")
	assert.False(t, a.ComposerEmpty(screen, 1, 3), "the rune count is not the width")
	assert.False(t, a.ComposerEmpty(screen, 1, 5))
	assert.False(t, a.ComposerEmpty(screen, 0, 4), "another row does not start with the prefix")
	assert.False(t, a.ComposerEmpty(screen, 3, 4), "past the last row")
	assert.False(t, a.ComposerEmpty(screen, -1, 4), "before the first row")
}

func TestComposerEmpty_ProviderDeclaringNoTypeBlockIsNeverEmpty(t *testing.T) {
	screen, row, col := composerRecording(t, "idle-fresh-80x24")

	assert.False(t, get(t, "codex").ComposerEmpty(screen, row, col))
}
