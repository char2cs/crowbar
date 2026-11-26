//go:build integration

package kit

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
)

const (
	bracketedPasteOn = "\x1b[?2004h"
	pasteOpen        = "\x1b[200~"
	pasteClose       = "\x1b[201~"
)

// composerPTY is a fake CLI in composer mode running on a real PTY the test types into.
type composerPTY struct {
	t      *testing.T
	ptmx   *os.File
	launch Launch
	mu     sync.Mutex
	out    []byte
	sig    *Signal
}

func startComposer(t *testing.T, c Composer) *composerPTY {
	t.Helper()
	var env []string
	for _, kv := range composerEnv(&c) {
		env = append(env, kv[0]+"="+kv[1])
	}
	return startFake(t, env...)
}

// startFake runs the fake CLI on a PTY with extra env, once its launch capture exists.
func startFake(t *testing.T, env ...string) *composerPTY {
	t.Helper()
	dir := t.TempDir()
	const seg = "seg1"
	cmd := exec.CommandContext(t.Context(), FakeCLIPath(t))
	cmd.Env = append(os.Environ(), "FAKECLI_DIR="+dir, "FAKECLI_SEG="+seg, "FAKECLI_TOKEN=tok")
	cmd.Env = append(cmd.Env, env...)
	ptmx, err := pty.Start(cmd)
	require.NoError(t, err)
	p := &composerPTY{t: t, ptmx: ptmx, sig: NewSignal()}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				p.mu.Lock()
				p.out = append(p.out, buf[:n]...)
				p.mu.Unlock()
				p.sig.Fire()
			}
			if err != nil {
				return
			}
		}
	}()
	p.launch = FakeProvider{Dir: dir}.AwaitLaunch(t, seg)
	return p
}

func (p *composerPTY) screen() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.out)
}

func (p *composerPTY) write(s string) {
	p.t.Helper()
	_, err := p.ptmx.WriteString(s)
	require.NoError(p.t, err)
}

func (p *composerPTY) await(what string, pred func(screen string) bool) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(p.t.Context(), 10*time.Second)
	defer cancel()
	Await(p.t, ctx, what, func() (string, bool) {
		s := p.screen()
		return s, pred(s)
	}, p.sig)
}

func (p *composerPTY) awaitCursor(col int) {
	p.t.Helper()
	p.await("cursor at column "+strconv.Itoa(col), func(s string) bool { return cursorCol(s) == col })
}

// cursorCol replays painted output far enough to know the cursor's column: absolute moves,
// carriage returns and the display width of printed text. Other escapes do not move it.
func cursorCol(raw string) int {
	col := 0
	rs := []rune(raw)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '[':
			j := i + 2
			for j < len(rs) && (rs[j] < '@' || rs[j] > '~') {
				j++
			}
			if j >= len(rs) {
				return col
			}
			if rs[j] == 'H' {
				_, c, _ := strings.Cut(string(rs[i+2:j]), ";")
				n, err := strconv.Atoi(c)
				if err != nil {
					n = 1
				}
				col = n - 1
			}
			i = j
		case r == '\r':
			col = 0
		case r >= ' ':
			col += uniseg.StringWidth(string(r))
		}
	}
	return col
}

func TestFakeComposer_PasteAndSubmitInOneWriteIsOneMessage(t *testing.T) {
	p := startComposer(t, Composer{})
	p.await("bracketed paste on", func(s string) bool { return strings.Contains(s, bracketedPasteOn) })
	p.awaitCursor(len("> "))

	p.write(pasteOpen + "hello there" + pasteClose + "\r")

	require.Equal(t, []string{"hello there"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_PasteThenSubmitInASecondWriteIsOneMessage(t *testing.T) {
	p := startComposer(t, Composer{})
	p.awaitCursor(len("> "))

	p.write(pasteOpen + "first" + pasteClose)
	p.awaitCursor(len("> first"))
	p.write("\r")
	require.Equal(t, []string{"first"}, p.launch.AwaitSubmitted(t, 1))

	// A second message arrives alone: the first one's draft was cleared by the submit.
	p.write(pasteOpen + "second" + pasteClose + "\r")
	require.Equal(t, []string{"first", "second"}, p.launch.AwaitSubmitted(t, 2))
}

func TestFakeComposer_SplitPasteMarkersStillPaste(t *testing.T) {
	p := startComposer(t, Composer{})
	p.awaitCursor(len("> "))

	// Markers cut across writes, with a carriage return inside the paste that must not submit.
	p.write(pasteOpen[:3])
	p.write(pasteOpen[3:] + "a\rb" + pasteClose[:2])
	p.write(pasteClose[2:] + "\r")

	require.Equal(t, []string{"a\rb"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_MultiLinePasteIsOneMessage(t *testing.T) {
	p := startComposer(t, Composer{})
	p.awaitCursor(len("> "))

	p.write(pasteOpen + "line one\nline two\n\nline four" + pasteClose + "\r")

	require.Equal(t, []string{"line one\nline two\n\nline four"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_TypedTextIsADraftThatMovesTheCursorPastThePrefix(t *testing.T) {
	p := startComposer(t, Composer{Prefix: "❯ "})
	p.awaitCursor(uniseg.StringWidth("❯ "))

	p.write("abc")
	p.awaitCursor(uniseg.StringWidth("❯ ") + 3)

	p.write("\r")
	require.Equal(t, []string{"abc"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_SeededDraftStartsWithTheCursorPastThePrefix(t *testing.T) {
	p := startComposer(t, Composer{Draft: "half written"})

	p.awaitCursor(len("> half written"))
	p.write("\r")
	require.Equal(t, []string{"half written"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_CaretHomeLeavesTheCursorBeforeTheDraft(t *testing.T) {
	p := startComposer(t, Composer{Draft: "half written", CaretHome: true})

	p.await("draft painted and cursor back after the prefix", func(s string) bool {
		return strings.Contains(s, "half written") && cursorCol(s) == len("> ")
	})
	p.write("\r")
	require.Equal(t, []string{"half written"}, p.launch.AwaitSubmitted(t, 1))
}

func TestFakeComposer_GhostTextIsPaintedButTheCursorStaysAfterThePrefix(t *testing.T) {
	p := startComposer(t, Composer{Ghost: "try asking about tests"})

	p.await("ghost painted and cursor back after the prefix", func(s string) bool {
		return strings.Contains(s, "try asking about tests") && cursorCol(s) == len("> ")
	})
}

func TestFakeComposer_BusyPaintsNoComposer(t *testing.T) {
	p := startComposer(t, Composer{Busy: true})

	p.await("bracketed paste on", func(s string) bool { return strings.Contains(s, bracketedPasteOn) })
	require.NotContains(t, p.screen(), "> ")
}
