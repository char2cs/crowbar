//go:build integration

package terminal_test

import (
	"regexp"
	"strings"
	"time"
)

// wrapScenario prints, on the 80-column terminal, a 120-column URL (the terminal soft-wraps
// it), an exactly 80-column row of x and a y row after it (the program hard-breaks them). The
// marker is assembled from a shell variable so it appears only in the output, never the echoed
// command.
const wrapScenario = "printf 'https://example.com/%0100d\\n' 0 | tr 0 a; printf '%080d\\n' 0 | tr 0 x; " +
	"echo yyyyyyyyyy; M=WRAP_DONE; echo \"${M}_7d1c\"; sleep 30\n"

var (
	wrapURL     = "https://example.com/" + strings.Repeat("a", 100)
	exactFitRow = strings.Repeat("x", 80)
	// A live frame reaches the row after an exact-fit row by a cursor address to column 1; an
	// autowrap continuation would follow the x row with no address at all.
	liveHardBreak = regexp.MustCompile(exactFitRow + "[^y]*?\x1b\\[[0-9]+;1H[^y]*?yyyyyyyyyy")
)

// TestRegression_TerminalSession_WrappedLinesStayUnbroken proves a URL the terminal width
// soft-wrapped reaches the client as one flow, in the live frames and in the re-attach
// redraw, while a line the program hard-broke after an exact-fit row is never joined.
func (s *TerminalSuite) TestRegression_TerminalSession_WrappedLinesStayUnbroken() {
	t := s.T()
	sessionID, ws1, lifecycle := s.newSessionWithLiveClient(t)

	ws1.SendJSON(t, map[string]any{"data": wrapScenario})
	var live strings.Builder
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		live.WriteString(d)
		return strings.Contains(live.String(), "WRAP_DONE_7d1c")
	})

	s.Contains(live.String(), wrapURL,
		"live frames must carry the soft-wrapped URL as one flow")
	// Live frames are cursor-addressed diffs, so their hard break is an address, never CRLF.
	s.NotContains(live.String(), exactFitRow+"yyyyyyyyyy",
		"live frames must not run an exact-fit hard-broken row into the next line")
	s.Regexp(liveHardBreak, live.String(),
		"live frames must address the row after an exact-fit hard break")

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws2.ReadMsg(t, 5*time.Second)
	s.Require().Equal(true, frame["snapshot"], "the first frame of a re-attach is the keyframe")
	redraw, _ := frame["data"].(string)
	s.Contains(redraw, wrapURL,
		"the re-attach redraw must carry the soft-wrapped URL as one flow")
	s.Contains(redraw, exactFitRow+"\r\nyyyyyyyyyy",
		"the re-attach redraw must keep an exact-fit hard break broken")

	s.killAndAwaitEnded(t, lifecycle, sessionID)
}
