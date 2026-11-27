package terminal

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const ShellPromptForTest = "<<CRWB-RDY>>"

func PinShellForTest(t *testing.T) {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", ShellPromptForTest)
	t.Setenv("ENV", "")
	t.Setenv("BASH_ENV", "")
}

func StripANSIForTest(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c != 0x1b {
			if c == '\n' || c >= 0x20 {
				out.WriteByte(c)
			}
			i++
			continue
		}
		i++
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[':
			i++
			for i < len(b) && (b[i] < '@' || b[i] > '~') {
				i++
			}
			i++
		case ']', 'P', 'X', '^', '_':
			i++
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case '(', ')', '*', '+':
			i += 2
		default:
			i++
		}
	}
	return out.String()
}

func ScreenTextForTest(eng Engine, sid string) string {
	return StripANSIForTest(SerializedForTest(eng, sid))
}

func WaitScreenForTest(
	t *testing.T,
	eng Engine,
	sid string,
	pred func(string) bool,
	what string,
) {
	t.Helper()
	notify := PumpNotifyForTest(eng, sid)
	require.NotNil(t, notify, "session %s must exist to wait on its pump", sid)
	done := SessionDoneForTest(eng, sid)
	for {
		if pred(ScreenTextForTest(eng, sid)) {
			return
		}
		select {
		case <-notify:
		case <-done:
			if pred(ScreenTextForTest(eng, sid)) {
				return
			}
			t.Fatalf("session %s exited before %s\nscreen:\n%s",
				sid, what, ScreenTextForTest(eng, sid))
		}
	}
}

func WaitPromptForTest(t *testing.T, eng Engine, sid string) {
	t.Helper()
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		return strings.Contains(s, ShellPromptForTest)
	}, "the shell to print its first prompt")
}

var markerSeq atomic.Int64

func RunShellForTest(t *testing.T, eng Engine, sid, cmd string) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("MK%dEND", n)
	line := fmt.Sprintf("%s; printf '%%s%%s\\n' MK%d END\n", cmd, n)
	require.NoError(t, eng.Write(context.Background(), sid, []byte(line)),
		"write %q to session %s", cmd, sid)
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		i := strings.LastIndex(s, tok)
		return i >= 0 && strings.Contains(s[i+len(tok):], ShellPromptForTest)
	}, fmt.Sprintf("command %q to finish and the prompt to return", cmd))
}

func StartForegroundForTest(t *testing.T, eng Engine, sid string) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("FG%dON", n)
	line := fmt.Sprintf("( printf '%%s%%s\\n' FG%d ON; exec sleep 9999 )\n", n)
	require.NoError(t, eng.Write(context.Background(), sid, []byte(line)),
		"start foreground child in session %s", sid)
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		return strings.Contains(s, tok)
	}, "the foreground child to announce itself")
	require.False(t, IsIdleForTest(eng, sid),
		"the child's first output byte must already prove it owns the terminal "+
			"(job control foregrounds a job's process group before it runs)")
}

func NewReadyShellForTest(t *testing.T, eng Engine, chatID, dir string) string {
	t.Helper()
	sid, err := eng.Create(context.Background(), chatID, dir, nil)
	require.NoError(t, err)
	WaitPromptForTest(t, eng, sid)
	return sid
}
