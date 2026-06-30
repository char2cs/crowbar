package model

import (
	"strconv"
	"strings"
)

// maxEscanParams bounds the CSI parameter bytes the in-Write scanner buffers for one
// sequence. DECSTBM carries at most "<top>;<bottom>" (a handful of bytes); anything past
// the cap is a pathological / non-DECSTBM CSI whose final byte we still consume to return
// to ground, but whose parameters we stop accumulating.
const maxEscanParams = 64

const (
	escGround = iota
	escEsc
	escSCSG0
	escSCSG1
	escCSI
)

// scanCharsetAndRegion runs a minimal, stateful in-Write scanner over p to recover the
// three pieces of terminal state the pinned x/vt commit does NOT surface through
// vt.Callbacks (its Callbacks struct has no Charset, LockingShift, or ScrollRegion field):
//
//   - SCS charset designation — ESC ( <d> designates G0, ESC ) <d> designates G1, recorded
//     in shadow.g0/g1 and re-emitted by serialize step 10.
//   - Locking shift — SI (0x0F) invokes G0 into GL, SO (0x0E) invokes G1 into GL, recorded
//     in shadow.glLock. A DEC line-drawing TUI commonly invokes G1 via SO; x/vt does NOT
//     itself resolve SO+G1 into the grid runes (verified at the pin), so without re-emitting
//     the shift the live post-attach stream mis-renders on the client.
//   - DECSTBM scroll region — CSI <top> ; <bottom> r, recorded in shadow.scrollTop/Bottom/
//     scrollRegionSet and re-emitted by serialize step 11 (guarded out when full-screen).
//
// It is the spec §4.1 contract-note / step-10 fallback for these missing callbacks. It
// recognises ONLY these sequences (never general escape semantics) and carries its state
// across Write calls, so a sequence split across PTY chunks is still recognised. The
// emulator parses the same bytes for its grid in parallel; this scanner only feeds the
// shadow state the serializer reads.
func (m *vtModel) scanCharsetAndRegion(
	p []byte,
) {
	for _, c := range p {
		m.escanStep(c)
	}
}

// resetEscan returns the scanner to ground state, dropping any in-flight partial sequence.
// Called after a parse-panic recreate, when the emulator is blanked to a known state.
func (m *vtModel) resetEscan() {
	m.escanState = escGround
	m.escanParams = m.escanParams[:0]
	m.escanPrivate = false
}

func (m *vtModel) escanStep(
	c byte,
) {
	switch m.escanState {
	case escGround:
		m.escanFromGround(c)
	case escEsc:
		m.escanFromEsc(c)
	case escSCSG0:
		m.escanFromSCS(c, 0)
	case escSCSG1:
		m.escanFromSCS(c, 1)
	default:
		m.escanFromCSI(c)
	}
}

func (m *vtModel) escanFromGround(
	c byte,
) {
	switch c {
	case 0x0e: // SO — invoke G1 into GL
		m.shadow.glLock = 1
	case 0x0f: // SI — invoke G0 into GL
		m.shadow.glLock = 0
	case 0x1b: // ESC
		m.escanState = escEsc
	case 0x9b: // 8-bit C1 CSI introducer
		m.beginCSI()
	}
}

func (m *vtModel) escanFromEsc(
	c byte,
) {
	switch c {
	case '(': // designate G0
		m.escanState = escSCSG0
	case ')': // designate G1
		m.escanState = escSCSG1
	case '[': // CSI
		m.beginCSI()
	default:
		// Any other ESC sequence (including CAN/SUB abort and the multi-byte SCS variants
		// ESC * / ESC + for G2/G3 we do not track) returns to ground; the emulator still
		// parses it for the grid.
		m.escanState = escGround
	}
}

// escanFromSCS records the charset designator for the given slot. Intermediate bytes
// (0x20-0x2f) of a multi-byte 96-charset designator are skipped; the final byte is recorded
// as the designator (the common single-byte designators 'B', '0', 'A', '1', '2' land here
// directly).
func (m *vtModel) escanFromSCS(
	c byte,
	slot int,
) {
	if c >= 0x20 && c <= 0x2f {
		return // intermediate; keep collecting
	}
	m.shadow.setCharset(slot, c)
	m.escanState = escGround
}

func (m *vtModel) beginCSI() {
	m.escanState = escCSI
	m.escanParams = m.escanParams[:0]
	m.escanPrivate = false
}

func (m *vtModel) escanFromCSI(
	c byte,
) {
	switch {
	case c == 0x18 || c == 0x1a: // CAN / SUB abort
		m.escanState = escGround
	case c >= 0x40 && c <= 0x7e: // final byte
		m.finishCSI(c)
		m.escanState = escGround
	case c >= 0x3c && c <= 0x3f: // private-marker byte (e.g. '?') — never a DECSTBM
		m.escanPrivate = true
		m.appendEscanParam(c)
	default: // parameter / intermediate byte
		m.appendEscanParam(c)
	}
}

func (m *vtModel) appendEscanParam(
	c byte,
) {
	if len(m.escanParams) >= maxEscanParams {
		return
	}
	m.escanParams = append(m.escanParams, c)
}

// finishCSI applies a completed CSI sequence. Only DECSTBM (final byte 'r', no private
// marker) is acted upon; every other CSI is ignored (x/vt handles it for the grid).
func (m *vtModel) finishCSI(
	final byte,
) {
	if final != 'r' || m.escanPrivate {
		return
	}
	m.applyDECSTBM(string(m.escanParams))
}

// applyDECSTBM updates the shadow scroll region from a DECSTBM parameter string. An empty
// parameter string (bare CSI r) resets the margins to full screen. A valid "<top>;<bottom>"
// (or "<top>" with bottom defaulting to the current height) sets a region; top must be
// >= 1 and strictly less than bottom, or the sequence is ignored (matching VT behavior).
func (m *vtModel) applyDECSTBM(
	params string,
) {
	if params == "" {
		m.shadow.scrollRegionSet = false
		m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
		return
	}
	top, bottom, ok := m.parseDECSTBM(params)
	if !ok {
		return
	}
	m.shadow.scrollTop, m.shadow.scrollBottom, m.shadow.scrollRegionSet = top, bottom, true
}

func (m *vtModel) parseDECSTBM(
	params string,
) (top int, bottom int, ok bool) {
	topStr := params
	botStr := ""
	if i := strings.IndexByte(params, ';'); i >= 0 {
		topStr, botStr = params[:i], params[i+1:]
		if strings.IndexByte(botStr, ';') >= 0 {
			return 0, 0, false // more than two parameters is not DECSTBM
		}
	}
	top = 1
	if topStr != "" {
		v, err := strconv.Atoi(topStr)
		if err != nil || v < 1 {
			return 0, 0, false
		}
		top = v
	}
	bottom = m.emu.Height()
	if botStr != "" {
		v, err := strconv.Atoi(botStr)
		if err != nil || v < 1 {
			return 0, 0, false
		}
		bottom = v
	}
	if top >= bottom {
		return 0, 0, false // a region must span at least two rows
	}
	return top, bottom, true
}
