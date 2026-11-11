package model

import (
	"bytes"
	"strconv"
	"strings"
)

// maxEscanParams bounds the CSI parameter bytes buffered per sequence: DECSTBM needs a handful;
// past the cap the final byte is still consumed but parameters stop accumulating.
const maxEscanParams = 64

// maxEscanOSCText bounds the OSC 0/1/2 body buffered per title, limiting growth on a never-terminated
// OSC; only trailing title text is ever truncated. The serializer caps separately.
const maxEscanOSCText = 4096

const (
	escGround = iota
	escEsc
	escSCSG0
	escSCSG1
	escCSI
	escOSC
	escOSCEsc
)

// scanCharsetAndRegion recovers the state the pinned x/vt gives no callback for (SCS designation,
// SI/SO locking shift, DECSTBM) plus OSC 0/1/2 titles, which x/vt truncates at a 0x9C UTF-8 byte.
// It recognises only these and carries state across Writes, so split sequences still match.
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
	m.escanOSC = m.escanOSC[:0]
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
	case escCSI:
		m.escanFromCSI(c)
	case escOSC:
		m.escanFromOSC(c)
	default: // escOSCEsc
		m.escanFromOSCEsc(c)
	}
}

// escanFromGround dispatches ground-state bytes. Only the 7-bit ESC starts a sequence: 0x9B is a
// valid UTF-8 continuation byte, and framing it as CSI would swallow the following text.
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
	case ']': // OSC — icon/title string
		m.beginOSC()
	case 'c': // RIS — full reset
		m.applyRIS()
		m.escanState = escGround
	default:
		// Any other ESC sequence (including CAN/SUB abort and the multi-byte SCS variants
		// ESC * / ESC + for G2/G3 we do not track) returns to ground; the emulator still
		// parses it for the grid.
		m.escanState = escGround
	}
}

// applyRIS resets the shadow state x/vt never reports back (scroll region, G0/G1, locking shift) on
// ESC c. Private modes and keypad are left to x/vt's resetModes callbacks: clearing them here would
// clobber an ESC = arriving in the same Write, since escan runs after the emulator.
func (m *vtModel) applyRIS() {
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
	m.shadow.g0, m.shadow.g1, m.shadow.glLock = 'B', 'B', 0
}

// escanFromSCS records the designator's final byte for the slot, skipping the intermediate bytes
// (0x20-0x2f) of a multi-byte 96-charset designator.
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
	case c == 0x1b: // ESC — abort this CSI and start a fresh sequence (defense-in-depth: a
		// real sequence arriving mid-CSI must not be swallowed as a param byte).
		m.escanState = escEsc
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

// applyDECSTBM updates the shadow scroll region: empty params reset to full screen; otherwise top
// must be >= 1 and below bottom (default: height) or the sequence is ignored, as on a VT.
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
) (top, bottom int, ok bool) {
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

// beginOSC starts an OSC body. The 8-bit 0x9D introducer is not honoured: it is a valid UTF-8
// continuation byte and would start an OSC on ordinary text.
func (m *vtModel) beginOSC() {
	m.escanState = escOSC
	m.escanOSC = m.escanOSC[:0]
}

// escanFromOSC accumulates the body until BEL or 7-bit ST (ESC \); CAN/SUB abort. Bytes >= 0x80,
// including 0x9C, are title content — the reason this scanner exists.
func (m *vtModel) escanFromOSC(
	c byte,
) {
	switch c {
	case 0x07: // BEL terminates
		m.finishOSC()
		m.escanState = escGround
	case 0x1b: // possible 7-bit ST (ESC \)
		m.escanState = escOSCEsc
	case 0x18, 0x1a: // CAN / SUB abort — discard the partial string
		m.escanState = escGround
	default:
		if len(m.escanOSC) < maxEscanOSCText {
			m.escanOSC = append(m.escanOSC, c)
		}
	}
}

// escanFromOSCEsc resolves the byte after an ESC inside an OSC string: ESC \ is the 7-bit ST
// terminator and commits the string; any other byte means that ESC was not a terminator, so
// the scanner returns to the string and treats the byte as content. A lone ESC is never itself appended, so it cannot corrupt the captured title.
func (m *vtModel) escanFromOSCEsc(
	c byte,
) {
	if c == '\\' {
		m.finishOSC()
		m.escanState = escGround
		return
	}
	m.escanState = escOSC
	m.escanFromOSC(c)
}

// finishOSC routes a "<code>;<text>" body to the title (0/2) and/or icon name (0/1); any other OSC
// is ignored. Text stays raw UTF-8; the serializer sanitizes it.
func (m *vtModel) finishOSC() {
	i := bytes.IndexByte(m.escanOSC, ';')
	if i < 0 {
		return
	}
	code, err := strconv.Atoi(string(m.escanOSC[:i]))
	if err != nil {
		return
	}
	text := string(m.escanOSC[i+1:])
	switch code {
	case 0:
		m.shadow.title = text
		m.shadow.iconName = text
	case 1:
		m.shadow.iconName = text
	case 2:
		m.shadow.title = text
	}
}
