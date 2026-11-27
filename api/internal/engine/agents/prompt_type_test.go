package agents_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
      input: keys
      chunk_bytes: 8
      chunk_interval_ms: 5
      submit: "\r"
      skip_prefixes: ["!", "/btw"]
      skip_suffixes: ["\\"]
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
		Input: "keys", Submit: "\r", ChunkBytes: 8, ChunkInterval: 5 * time.Millisecond, ComposerRowPrefix: "❯ ", SkipPrefixes: []string{"!", "/btw"}, SkipSuffixes: []string{"\\"},
	}, typing, "the row prefix keeps its no-break space")

	_, ok = get(t, "codex").PromptType()
	assert.False(t, ok, "a provider that declares no type block is never typed into")
}

func TestAgent_ClaudeDeclaresTypedDeliveryWithItsNoBreakSpacePrefix(t *testing.T) {
	typing, ok := get(t, "claude").PromptType()

	require.True(t, ok)
	assert.Equal(t, agents.PromptTyping{
		Input: "keys", Submit: "\r", ChunkBytes: 256, ChunkInterval: 10 * time.Millisecond, ComposerRowPrefix: "❯ ", ComposerGhost: "faint", SkipPrefixes: []string{"/", "!"}, SkipSuffixes: []string{"\\"},
	}, typing)
	assert.Equal(t, []byte{0xe2, 0x9d, 0xaf, 0xc2, 0xa0}, []byte(typing.ComposerRowPrefix))
}

func TestAgent_PromptTypeWritesChunkTheMessageThenSubmit(t *testing.T) {
	a := typist(t)

	got, ok := a.PromptTypeWrites("first line\nsecond \"line\"")
	require.True(t, ok)
	assert.Equal(t, []string{"first li", "ne\nsecon", "d \"line\"", "\r"}, writeStrings(got))

	got, ok = a.PromptTypeWrites("")
	require.True(t, ok)
	assert.Equal(t, []string{"\r"}, writeStrings(got), "an empty message is only the submit key")
}

func TestAgent_PromptTypeWritesNeverSplitARune(t *testing.T) {
	got, ok := typist(t).PromptTypeWrites("aaaaaaé日本語")
	require.True(t, ok)
	var joined strings.Builder
	for _, w := range got[:len(got)-1] {
		assert.True(t, utf8.Valid(w), "%q", w)
		assert.LessOrEqual(t, len(w), 8)
		joined.Write(w)
	}
	assert.Equal(t, "aaaaaaé日本語", joined.String())
}

func writeStrings(writes [][]byte) []string {
	out := make([]string, len(writes))
	for i, w := range writes {
		out[i] = string(w)
	}
	return out
}

func TestAgent_PromptTypeWritesRefuseWhatTheCLIWouldActOnOrIsACommand(t *testing.T) {
	a := typist(t)

	for name, text := range map[string]string{
		"an escape sequence":             "hi\x1b[201~rm -rf /",
		"a carriage return":              "hi\rthere",
		"a tab":                          "a\tb",
		"a message the type block skips": "!ls",
		"a longer skipped prefix":        "/btw and so on",
		"a C1 control (NEL)":             "hi\u0085there",
		"a C1 control (CSI)":             "hi\u009b[201~",
		"a trailing backslash":           "see the path C:\\",
		"a backslash ending a last line": "one\ntwo \\",
	} {
		got, ok := a.PromptTypeWrites(text)
		assert.False(t, ok, name)
		assert.Empty(t, got, name)
	}

	got, ok := a.PromptTypeWrites("/compact")
	require.True(t, ok, "only the type block's own skip list applies, not steer's")
	assert.Equal(t, []string{"/compact", "\r"}, writeStrings(got))

	_, ok = get(t, "codex").PromptTypeWrites("hello")
	assert.False(t, ok, "a provider that declares no type block renders nothing")
}

func TestAgent_CanTypeIsFalseForSkippedOrUnsafeMessages(t *testing.T) {
	a := typist(t)
	assert.True(t, a.CanType("hello\nworld"))
	assert.True(t, a.CanType("not a !command"))
	assert.False(t, a.CanType("!ls"))
	assert.False(t, a.CanType("/btw hi"))
	assert.False(t, a.CanType("x\x1b[201~"))
	assert.True(t, a.CanType("a backslash \\ mid-text and \\ before a space \\ "), "only the last character can join a line")
	assert.True(t, a.CanType("日本語 é\u00a0"), "text just past the C1 range is plain")
	assert.False(t, get(t, "codex").CanType("hello"))
}

func composerRecording(t *testing.T, state string) agents.ComposerScreen {
	t.Helper()
	base := "testdata/claude-composer-" + state + "-2.1.325"
	text, err := os.ReadFile(base + ".screen")
	require.NoError(t, err)
	cursor, err := os.ReadFile(base + ".cursor")
	require.NoError(t, err)
	screen := agents.ComposerScreen{Text: string(text)}
	var paste bool
	_, err = fmt.Sscanf(string(cursor), "%d %d %t", &screen.CursorRow, &screen.CursorCol, &paste)
	require.NoError(t, err)
	mask, err := os.ReadFile(base + ".faint")
	if errors.Is(err, os.ErrNotExist) {
		return screen
	}
	require.NoError(t, err)
	for _, row := range strings.Split(string(mask), "\n") {
		screen.Faint = append(screen.Faint, faintRow(row))
	}
	return screen
}

func faintRow(mask string) []bool {
	row := make([]bool, len(mask))
	for i := range mask {
		row[i] = mask[i] == 'f'
	}
	return row
}

func typistWith(t *testing.T, prefix, ghost string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	composer := "row_prefix: " + strconv.Quote(prefix)
	if ghost != "" {
		composer += "\n        ghost: " + ghost
	}
	body := strings.Replace(typedPromptDescriptor, "row_prefix: \"❯ \"", composer, 1)
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
		{"midturn-80x24", true},
		{"draft-80x24", false},
		{"draft-120x40", false},
		{"draft-home-80x24", false},
		{"draft-home-120x40", false},
		{"ghost-draft-home-80x24", false},
		{"help-dialog-80x24", false},
		{"paste-bang-80x24", false},
		{"paste-escaped-bang-80x24", false},
		{"paste-multiline-80x24", false},
		{"slash-typed-80x24", false},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			assert.Equal(t, tc.empty, get(t, "claude").ComposerEmpty(composerRecording(t, tc.state)))
		})
	}
}

func TestComposerEmpty_APlainSpacePrefixNeverMatchesTheNoBreakSpaceRecording(t *testing.T) {
	screen := composerRecording(t, "idle-fresh-80x24")

	assert.False(t, typistWith(t, "❯ ", "faint").ComposerEmpty(screen))
	assert.True(t, typistWith(t, "❯ ", "faint").ComposerEmpty(screen))
}

func TestComposerEmpty_WithNoGhostDeclaredAnyTextAfterTheCursorIsADraft(t *testing.T) {
	a := typistWith(t, "❯ ", "")

	assert.False(t, a.ComposerEmpty(composerRecording(t, "idle-ghost-80x24")), "faint text is a draft unless declared ghost")
	assert.True(t, a.ComposerEmpty(composerRecording(t, "midturn-80x24")), "a blank rest of row is empty")
}

func TestComposerEmpty_TextAfterTheCursorMustAllBeFaint(t *testing.T) {
	a := typistWith(t, "> ", "faint")
	screen := agents.ComposerScreen{Text: "> ab  日x", CursorRow: 0, CursorCol: 2}

	screen.Faint = [][]bool{faintRow("..ff..ff.")}
	assert.False(t, a.ComposerEmpty(screen), "a cell after a wide glyph is still a cell of the row")
	screen.Faint = [][]bool{faintRow("..f..fff")}
	assert.False(t, a.ComposerEmpty(screen), "one plain letter is a draft")
	screen.Faint = [][]bool{faintRow("..ff..fff")}
	assert.True(t, a.ComposerEmpty(screen), "blanks need not be faint")
	screen.Faint = [][]bool{faintRow("..ff")}
	assert.False(t, a.ComposerEmpty(screen), "a cell past the mask is not faint")
}

func TestComposerEmpty_JudgesTheCursorRowAgainstThePrefixDisplayWidth(t *testing.T) {
	a := typistWith(t, "あ> ", "faint")
	at := func(row, col int) agents.ComposerScreen {
		return agents.ComposerScreen{Text: "banner\nあ> \nfooter", CursorRow: row, CursorCol: col}
	}

	assert.True(t, a.ComposerEmpty(at(1, 4)), "a wide glyph takes two cells: 2 + '>' + ' '")
	assert.False(t, a.ComposerEmpty(at(1, 3)), "the rune count is not the width")
	assert.False(t, a.ComposerEmpty(at(1, 5)))
	assert.False(t, a.ComposerEmpty(at(0, 4)), "another row does not start with the prefix")
	assert.False(t, a.ComposerEmpty(at(3, 4)), "past the last row")
	assert.False(t, a.ComposerEmpty(at(-1, 4)), "before the first row")
}

func TestComposerEmpty_ProviderDeclaringNoTypeBlockIsNeverEmpty(t *testing.T) {
	assert.False(t, get(t, "codex").ComposerEmpty(composerRecording(t, "idle-fresh-80x24")))
}
