//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// shellStopExtraEvents add a background-command tool and, optionally, the
// descriptor's own words for asking the agent to end one.
const shellStopToolEvents = `events:
  tool_post:
    in: tool_post
    kind_map: { Run: execute, Halt: meta }
    map:
      session_id: session_id
      tool_id: tool_use_id
      tool_name: tool_name
      tool_kind: tool_name
      tool_target: command
      spawns_shell: bg
      stops_shell: stop
`

const shellStopEvent = `  shell_stop:
    out: prompt
    send: { text: "End background task {shell_id} now." }
`

func newShellStopFixture(t *testing.T, declaresStop bool) steerFixture {
	t.Helper()
	h := newHarness(t)
	f := steerFixture{h: h, provider: "shellstopstub", launches: filepath.Join(t.TempDir(), "launches")}
	yaml := fmt.Sprintf(steerStubDescriptorYAML, f.provider, f.launches, kit.FakeCLIPath(t))
	extra := shellStopToolEvents
	if declaresStop {
		extra += shellStopEvent
	}
	yaml = strings.Replace(yaml, "events:\n", extra, 1)
	writeProviderDescriptor(t, h, f.provider, yaml)
	f.imported = importWritableWorkspace(t, h)
	f.chatID, f.runnerID = createStubChat(t, h, f.imported, f.provider)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"serve it"}`)
	f.hook(t, "tool_post", `{"session_id":"sid-1","tool_use_id":"c1","tool_name":"Run","command":"npm run dev","bg":"sh-1"}`)
	return f
}

func (f steerFixture) stopPath(shellID string) string {
	return repoBase(f.imported) + "/chats/" + f.chatID + "/shells/" + shellID + "/stop"
}

func (f steerFixture) shell(t *testing.T, id string) ledger.Shell {
	t.Helper()
	shell, ok := chatState(t, f.h, f.chatID).ShellOf(id)
	require.True(t, ok)
	return shell
}

// Asking a running shell to end sends the agent the descriptor's request and
// marks the shell stopping; it does not end it, and asking again says nothing more.
func TestRegression_StoppingAShellAsksTheAgentAndWaitsForItsEnd(t *testing.T) {
	f := newShellStopFixture(t, true)

	f.h.post(f.stopPath("sh-1"), nil, http.StatusAccepted, nil)

	shell := f.shell(t, "sh-1")
	assert.True(t, shell.Live(), "the shell is not closed until the agent really ends it")
	assert.True(t, shell.Stopping)
	f.h.post(f.stopPath("sh-1"), nil, http.StatusAccepted, nil)
	reply := f.hook(t, "turn_stop", `{"session_id":"sid-1","last_assistant_message":"ok"}`)
	assert.JSONEq(t, `{"decision":"block","reason":"STEERED: End background task sh-1 now."}`, reply)
	assert.Equal(t, 1, strings.Count(fmt.Sprint(readRecordedMessages(t, f.h, f.imported, f.chatID)), "End background task"),
		"a second ask sends nothing more")

	f.hook(t, "tool_post", `{"session_id":"sid-1","tool_use_id":"c2","tool_name":"Halt","stop":"sh-1"}`)
	assert.Equal(t, ledger.ShellStopped, f.shell(t, "sh-1").Status)
}

func TestRegression_StoppingAShellThatIsNotRunningIsRefused(t *testing.T) {
	f := newShellStopFixture(t, true)
	f.h.postError(f.stopPath("nope"), nil, http.StatusConflict)

	f.hook(t, "tool_post", `{"session_id":"sid-1","tool_use_id":"c2","tool_name":"Halt","stop":"sh-1"}`)
	f.h.postError(f.stopPath("sh-1"), nil, http.StatusConflict)
}

func TestRegression_AProviderWithNoShellStopOffersNoStop(t *testing.T) {
	f := newShellStopFixture(t, false)
	f.h.postError(f.stopPath("sh-1"), nil, http.StatusNotFound)

	var providers []struct {
		ID        string `json:"id"`
		ShellStop bool   `json:"shellStop"`
	}
	f.h.get(repoBase(f.imported)+"/chats/providers", &providers)
	for _, p := range providers {
		if p.ID == f.provider {
			assert.False(t, p.ShellStop)
		}
	}
}

func TestRegression_AProviderWithShellStopSaysSo(t *testing.T) {
	f := newShellStopFixture(t, true)
	var providers []struct {
		ID        string `json:"id"`
		ShellStop bool   `json:"shellStop"`
	}
	f.h.get(repoBase(f.imported)+"/chats/providers", &providers)
	for _, p := range providers {
		if p.ID == f.provider {
			assert.True(t, p.ShellStop)
			return
		}
	}
	require.Fail(t, "provider not listed")
}
