package model

import (
	"bytes"
	"strconv"
	"strings"
)

// maxEscanParams bounds the CSI parameter bytes the in-Write scanner buffers for one
// sequence. DECSTBM carries at most "<top>;<bottom>" (a handful of bytes); anything past
// the cap is a pathological / non-DECSTBM CSI whose final byte we still consume to return
// to ground, but whose parameters we stop accumulating.
const maxEscanParams = 64

// maxEscanOSCText bounds the OSC 0/1/2 body bytes the in-Write scanner buffers for one
// title/icon sequence. A real window title is far shorter; the cap bounds growth on a
// never-terminated OSC (the parameter number lives at the head of the body, so it is never
// lost to the cap — only the trailing title text is truncated). The serializer independently
// caps the emitted title at maxOSCTextRunes.
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

// scanCharsetAndRegion runs a minimal, stateful in-Write scanner over p to recover the
// three pieces of terminal state the pinned x/vt commit does NOT surface through
// vt.Callbacks (its Callbacks struct has no Charset, LockingShift, or ScrollRegion field):
//
//   - SCS charset designation — ESC ( <d> designates G0, ESC ) <d> designates G1, recorded
//     in shadow.g0/g1 and re-emitted by serialize step 10.
//   - Locking shift — SI (0x0F) invokes G0 into GL, SO (0x0E) invokes G1 into GL, recorded
//     in shadow.glLock. A DEC line-drawing TUI commonly invokes G1 via SO; x/vt does NOT
//     itself resolve SO+G1 into the grid runes (verified at the pin), so without re-emitting
//     the shift the live post-attach stream mis-renders on the client. Re-emitting fixes only
//     that FUTURE live output; grid cells already painted via the unresolved G1 path self-heal
//     on the app's next repaint rather than being reproduced exactly on attach.
//   - DECSTBM scroll region — CSI <top> ; <bottom> r, recorded in shadow.scrollTop/Bottom/
//     scrollRegionSet and re-emitted by serialize step 11 (guarded out when full-screen).
//   - OSC 0/1/2 window title / icon name — ESC ] <0|1|2> ; <text> BEL|ST, recorded in
//     shadow.title / shadow.iconName. x/vt DOES surface these via its Title/IconName
//     callbacks, but its OSC-string parser honours the 8-bit C1 ST (0x9C) as a terminator,
//     and 0x9C is also a valid UTF-8 continuation byte (e.g. the sparkle "✳" U+2733 = E2 9C
//     B3), so a UTF-8 title carrying that byte is TRUNCATED at the 0x9C by x/vt. This scanner
//     is UTF-8-transparent — it terminates only on BEL (0x07) or the 7-bit ST (ESC \), never
//     on any byte >= 0x80 — so it captures the full title where x/vt's callback cannot. Its
//     value is authoritative: the Title/IconName callbacks are dropped from buildEmu.
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

// escanFromGround dispatches ground-state bytes. Only the 7-bit ESC introduces a sequence:
// the 8-bit C1 CSI introducer 0x9B is deliberately NOT honoured, because the emulator is fed
// a UTF-8 byte stream where 0x9B is a valid continuation byte (e.g. '‛' U+201B = E2 80 9B),
// and framing it as a CSI start would swallow the following bytes — the identical UTF-8
// mis-framing class commit 94d2da39 removed from scanPartial and beginOSC excludes for the
// sibling 0x9D OSC introducer. Real apps emit the 7-bit ESC[ CSI form.
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

// applyRIS mirrors x/vt's fullReset (ESC c) for exactly the shadow state x/vt never reports
// back: it resets the scroll region and the G0/G1 charset designation + active locking shift
// to hardware defaults. The DEC private modes are NOT touched here — x/vt's resetModes
// re-applies every default through the EnableMode/DisableMode callbacks, so shadow.modes is
// already corrected by the time RIS returns. Application-keypad (DEC-66) is likewise corrected
// by that resetModes callback (DisableMode(66)), so it is deliberately NOT cleared here:
// clearing it would clobber a DECKPAM (ESC =) that arrives in the SAME Write chunk as the RIS,
// because escan runs as a second pass AFTER the emulator callbacks have already observed the
// ESC = (escan does not itself re-observe ESC =). Like the resize reset, this prevents a later
// Serialize from re-emitting a stale DECSTBM / charset the app has already cleared.
func (m *vtModel) applyRIS() {
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
	m.shadow.g0, m.shadow.g1, m.shadow.glLock = 'B', 'B', 0
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

// beginOSC starts accumulating an OSC (Operating System Command) body. Only the 7-bit ESC ]
// introducer starts an OSC here; the 8-bit C1 OSC introducer (0x9D) is deliberately NOT
// honoured, because — like the 0x9C ST this scanner exists to survive — 0x9D is a valid
// UTF-8 continuation byte (e.g. 'Н' U+041D = D0 9D) and honouring it as an introducer would
// spuriously start an OSC on ordinary text. Real apps emit the 7-bit ESC ] form.
func (m *vtModel) beginOSC() {
	m.escanState = escOSC
	m.escanOSC = m.escanOSC[:0]
}

// escanFromOSC accumulates the OSC body until a UTF-8-safe terminator. BEL (0x07) commits the
// string; ESC (0x1B) may begin a 7-bit ST (ESC \) so it defers to escanFromOSCEsc; CAN/SUB
// abort and discard. Every other byte — crucially INCLUDING the 8-bit ST 0x9C and all other
// bytes >= 0x80 — is title content, appended up to maxEscanOSCText. This is the whole point:
// x/vt's own OSC parser truncates a UTF-8 title at an embedded 0x9C; this one does not.
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
// the scanner returns to the string and treats the byte as content (matching the pending-input
// framing). A lone ESC is never itself appended, so it cannot corrupt the captured title.
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

// finishOSC parses a completed OSC body of the form "<code>;<text>" and routes the text to the
// shadow title (code 0/2) and/or icon name (code 1/0). A body with no ';' separator or a
// non-numeric code is not a title/icon OSC (e.g. OSC 8 hyperlink, OSC 10/11/12 colour, OSC 52
// clipboard) and is ignored. The stored text is raw UTF-8; the serializer sanitizes it.
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
