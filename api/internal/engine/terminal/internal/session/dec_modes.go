package session

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
)

// trackedDECModes are the DEC private modes whose live on/off state must survive a
// client re-attach (workspace switch, daemon restart). xterm rebuilds these from
// the replayed scrollback, which is unreliable: the one-time SET sequence an app
// emits at startup can be evicted from the bounded ring, or removed by the replay
// sanitizer. So the session tracks them from the PTY output and re-asserts the
// net-active set as a preamble on Attach (see Session.Attach / decModeTracker).
//
//	1               application cursor keys (DECCKM) — arrows emit SS3, which TUIs
//	                (Claude Code) need to treat Up/Down as transcript navigation
//	1000/1002/1003  mouse tracking (click / button-event / any-event)
//	1006/1015       mouse report encodings (SGR / urxvt)
//	1004            focus reporting
//	2004            bracketed paste
//	47/1047/1049    alternate screen
var trackedDECModes = map[int]struct{}{
	1:    {},
	1000: {}, 1002: {}, 1003: {},
	1006: {}, 1015: {},
	1004: {},
	2004: {},
	47: {}, 1047: {}, 1049: {},
}

var (
	decPrivPrefix = []byte("\x1b[?")
	// ESC [ ? <params> (h|l) — DECSET/DECRST. Params may be ';'-separated.
	decModeRe = regexp.MustCompile(`\x1b\[\?([0-9;]+)([hl])`)
)

// decModeTracker keeps the live on/off state of the tracked DEC private modes.
// It is NOT internally synchronized: the owning Session calls observe()/preamble()
// only while holding s.mu.
type decModeTracker struct {
	on      map[int]bool
	pending []byte // trailing incomplete ESC[? sequence carried across PTY chunks
}

func newDecModeTracker() *decModeTracker { return &decModeTracker{on: make(map[int]bool)} }

// observe scans one PTY output chunk for DECSET/DECRST (ESC [ ? <params> h|l) of
// tracked modes and updates their state. A small residual is carried across chunk
// boundaries so a mode sequence split between two PTY reads is still detected.
func (t *decModeTracker) observe(chunk []byte) {
	if t == nil {
		return
	}
	buf := chunk
	if len(t.pending) > 0 {
		buf = append(append([]byte(nil), t.pending...), chunk...)
		t.pending = t.pending[:0]
	}
	// Cheap native-substring bail-out: terminal output rarely contains a DEC
	// private-mode sequence, so skip the regex on the common path.
	if !bytes.Contains(buf, decPrivPrefix) {
		return
	}
	lastEnd := 0
	for _, m := range decModeRe.FindAllSubmatchIndex(buf, -1) {
		lastEnd = m[1]
		set := buf[m[4]] == 'h' // group 2: the final 'h' (set) or 'l' (reset) byte
		for _, p := range bytes.Split(buf[m[2]:m[3]], []byte{';'}) {
			n, err := strconv.Atoi(string(p))
			if err != nil {
				continue
			}
			if _, ok := trackedDECModes[n]; ok {
				t.on[n] = set
			}
		}
	}
	// Carry a trailing, still-incomplete `ESC [ ? <params>` (no h/l terminator yet)
	// to the next chunk so a split sequence is not lost.
	if i := bytes.LastIndex(buf[lastEnd:], decPrivPrefix); i >= 0 {
		tail := buf[lastEnd+i:]
		if !bytes.ContainsAny(tail, "hl") && len(tail) <= 24 {
			t.pending = append(t.pending[:0], tail...)
		}
	}
}

// preamble returns a DECSET sequence re-asserting every currently-on tracked mode
// (e.g. "\x1b[?1h\x1b[?1000h\x1b[?1006h"), or nil if none are on. Modes that are
// off are omitted — a freshly-created xterm already defaults them off. Output is
// sorted for determinism.
func (t *decModeTracker) preamble() []byte {
	if t == nil {
		return nil
	}
	modes := make([]int, 0, len(t.on))
	for n, on := range t.on {
		if on {
			modes = append(modes, n)
		}
	}
	if len(modes) == 0 {
		return nil
	}
	sort.Ints(modes)
	var b []byte
	for _, n := range modes {
		b = append(b, decPrivPrefix...)
		b = strconv.AppendInt(b, int64(n), 10)
		b = append(b, 'h')
	}
	return b
}
