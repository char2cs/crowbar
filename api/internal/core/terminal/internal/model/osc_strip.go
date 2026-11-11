package model

import "strconv"

// OSC 0/1/2 strip: x/vt ends an OSC on the 8-bit ST 0x9C, a UTF-8 continuation byte, and prints a
// title's leftover bytes into the grid. escan already captures titles, so these OSCs are removed before
// the emulator with escan's framing (BEL or ESC \ only) and its Atoi code parse; other OSCs pass.

const (
	stripGround = iota
	stripEsc
	stripOSCCode
	stripOSCDiscard
	stripOSCDiscardEsc
	stripOSCPass
	stripOSCPassEsc
)

// stripOSCTitles returns p minus every OSC 0/1/2, keeping state across Writes: the possible
// introducer is WITHHELD until the code decides strip (discard) or keep (flush).
func (m *vtModel) stripOSCTitles(
	p []byte,
) []byte {
	out := make([]byte, 0, len(p))
	for _, c := range p {
		out = m.stripStep(out, c)
	}
	return out
}

// resetStrip returns the strip machine to ground, dropping any withheld partial OSC. Called
// from recreateEmu after a parse-panic recreate, alongside resetEscan, so the fresh emulator
// starts from a clean, agreed state.
func (m *vtModel) resetStrip() {
	m.stripState = stripGround
	m.stripWithheld = m.stripWithheld[:0]
	m.stripCode = m.stripCode[:0]
}

func (m *vtModel) stripStep(
	out []byte,
	c byte,
) []byte {
	switch m.stripState {
	case stripGround:
		return m.stripFromGround(out, c)
	case stripEsc:
		return m.stripFromEsc(out, c)
	case stripOSCCode:
		return m.stripFromOSCCode(out, c)
	case stripOSCDiscard:
		return m.stripFromDiscard(out, c)
	case stripOSCDiscardEsc:
		return m.stripFromDiscardEsc(out, c)
	case stripOSCPass:
		return m.stripFromPass(out, c)
	default: // stripOSCPassEsc
		return m.stripFromPassEsc(out, c)
	}
}

// stripFromGround withholds only the 7-bit ESC, which may start an OSC; every other byte,
// including the 8-bit C1 OSC (a UTF-8 continuation byte), is content, matching escan.
func (m *vtModel) stripFromGround(
	out []byte,
	c byte,
) []byte {
	if c == 0x1b { // ESC — might be ESC ] (OSC); withhold until the next byte decides
		m.stripWithheld = append(m.stripWithheld[:0], c)
		m.stripState = stripEsc
		return out
	}
	return append(out, c)
}

// stripFromEsc: ESC ] starts an OSC; anything else flushes the withheld ESC and reprocesses
// the byte from ground, so CSI and other escapes reach the emulator intact.
func (m *vtModel) stripFromEsc(
	out []byte,
	c byte,
) []byte {
	if c == ']' { // OSC introducer
		m.stripWithheld = append(m.stripWithheld, c)
		m.stripCode = m.stripCode[:0]
		m.stripState = stripOSCCode
		return out
	}
	out = m.flushWithheld(out)
	m.stripState = stripGround
	return m.stripStep(out, c)
}

// stripFromOSCCode accumulates every leading digit (escan parses the whole field with Atoi, so
// "00" is 0) while still withholding; the first non-digit decides discard (title) or flush.
func (m *vtModel) stripFromOSCCode(
	out []byte,
	c byte,
) []byte {
	if c >= '0' && c <= '9' {
		m.stripWithheld = append(m.stripWithheld, c)
		m.stripCode = append(m.stripCode, c)
		return out
	}
	if m.oscCodeIsTitle() {
		m.stripWithheld = m.stripWithheld[:0]
		m.stripState = stripOSCDiscard
		return m.stripFromDiscard(out, c)
	}
	out = m.flushWithheld(out)
	m.stripState = stripOSCPass
	return m.stripFromPass(out, c)
}

// oscCodeIsTitle uses the IDENTICAL Atoi parse as escan.finishOSC (0/1/2 and "00"/"01"/"02"), so
// strip and capture never disagree on which OSCs are titles.
func (m *vtModel) oscCodeIsTitle() bool {
	code, err := strconv.Atoi(string(m.stripCode))
	if err != nil {
		return false
	}
	return code == 0 || code == 1 || code == 2
}

// stripFromDiscard drops an OSC 0/1/2 body until BEL, a 7-bit ST, or CAN/SUB; 0x9C and every
// byte >= 0x80 is title content, never a terminator.
func (m *vtModel) stripFromDiscard(
	out []byte,
	c byte,
) []byte {
	switch c {
	case 0x07: // BEL terminates
		m.stripState = stripGround
	case 0x1b: // possible 7-bit ST (ESC \)
		m.stripState = stripOSCDiscardEsc
	case 0x18, 0x1a: // CAN / SUB abort
		m.stripState = stripGround
	}
	return out
}

// stripFromDiscardEsc resolves the byte after an ESC inside a discarded OSC: ESC \ is the
// 7-bit ST terminator; any other byte means that ESC was not a terminator, so we return to
// discarding and drop the byte as content (mirroring escan's OSC framing). Nothing is emitted.
func (m *vtModel) stripFromDiscardEsc(
	out []byte,
	c byte,
) []byte {
	if c == '\\' { // ST — end of the stripped OSC
		m.stripState = stripGround
		return out
	}
	m.stripState = stripOSCDiscard
	return m.stripFromDiscard(out, c)
}

// stripFromPass emits the body of a kept OSC (code >= 10, empty, or otherwise non-title)
// straight through to the emulator, watching for the terminator so the machine returns to
// ground for the NEXT sequence: BEL/CAN/SUB end it, ESC may begin a 7-bit ST.
func (m *vtModel) stripFromPass(
	out []byte,
	c byte,
) []byte {
	out = append(out, c)
	switch c {
	case 0x07, 0x18, 0x1a: // BEL terminates; CAN/SUB abort
		m.stripState = stripGround
	case 0x1b: // possible 7-bit ST (ESC \)
		m.stripState = stripOSCPassEsc
	}
	return out
}

// stripFromPassEsc resolves the byte after an ESC inside a kept OSC. ESC \ is the ST
// terminator (emit and return to ground); any other byte was not a terminator, so it is
// re-processed as pass-through content. The ESC itself was already emitted by stripFromPass.
func (m *vtModel) stripFromPassEsc(
	out []byte,
	c byte,
) []byte {
	if c == '\\' { // ST — end of the kept OSC
		out = append(out, c)
		m.stripState = stripGround
		return out
	}
	m.stripState = stripOSCPass
	return m.stripFromPass(out, c)
}

// flushWithheld appends the buffered, ambiguous OSC-introducer bytes to out and clears the
// buffer — used once a withheld OSC turns out to be one we keep (pass through to x/vt).
func (m *vtModel) flushWithheld(
	out []byte,
) []byte {
	out = append(out, m.stripWithheld...)
	m.stripWithheld = m.stripWithheld[:0]
	return out
}
