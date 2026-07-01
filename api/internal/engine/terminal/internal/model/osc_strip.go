package model

// OSC 0/1/2 (icon name / window title) strip state machine.
//
// Motivation: the pinned x/vt commit parses its input as a byte stream and honours the 8-bit
// C1 String Terminator (0x9C) as an OSC terminator. 0x9C is ALSO a valid UTF-8 continuation
// byte (the sparkle "✳" U+2733 = E2 9C B3), so when a UTF-8 title carrying that byte reaches
// x/vt's OSC-string parser it EXITS the OSC after "\xe2", drops to ground, and PRINTS the
// leftover title bytes (" Claude Code") into the grid as visible cells at the cursor. A
// Claude spinner re-sets its title constantly, so "Claude Code" gets stamped across the grid;
// our serialize then faithfully reproduces that corruption on re-attach.
//
// We already capture the title ourselves, UTF-8-transparently, via the escan scanner
// (escan.go) — so x/vt does not need to see OSC 0/1/2 at all. This machine removes exactly
// the OSC 0/1/2 sequences from the byte stream BEFORE it reaches the emulator, using the
// SAME UTF-8-aware framing escan uses to capture them (terminate only on BEL 0x07 or the
// 7-bit ST ESC \, NEVER on a lone 0x9C or any byte >= 0x80). It is stateful across Write
// calls so an OSC split across PTY chunks is still recognised and never leaks a dangling
// partial OSC title into x/vt.
//
// It strips ONLY OSC 0/1/2. OSC 10/11/12 (default colours, code "10"/"11"/"12"), OSC 4, OSC 8
// and every other OSC pass straight through to x/vt unchanged, because a code with two or
// more digits can never be 0/1/2. The live-client fan-out is not involved here — the session
// fans out the RAW stream to clients; only what the EMULATOR sees is stripped.
//
// Because a conforming terminal moves no cursor on an OSC, removing the sequence also removes
// the spurious cursor movement the mis-print caused.

const (
	stripGround = iota
	stripEsc
	stripOSCCode
	stripOSCDiscard
	stripOSCDiscardEsc
	stripOSCPass
	stripOSCPassEsc
)

// stripOSCTitles returns the bytes of p that should reach the emulator with every OSC 0/1/2
// sequence removed. It advances the persistent strip state (m.stripState / m.stripWithheld /
// m.stripCode) so a sequence split across Write chunks is handled correctly: the bytes before
// an OSC start feed the emulator immediately, the ambiguous OSC-introducer bytes are WITHHELD
// until the code disambiguates strip-vs-keep, and only then are they discarded (OSC 0/1/2) or
// flushed through (any other OSC).
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

// stripFromGround emits ordinary bytes straight through. Only the 7-bit ESC is withheld,
// because it MIGHT introduce an OSC (ESC ]) we have to strip; every other introducer form
// (including the 8-bit C1 OSC 0x9D, a UTF-8 continuation byte we never treat as a control)
// is content and passes through, matching escan's 7-bit-only framing.
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

// stripFromEsc resolves the byte after a withheld ESC. ESC ] begins an OSC (keep withholding,
// start reading the code). Anything else means this was not an OSC: flush the withheld ESC and
// reprocess the byte from ground, so ESC ESC re-arms the withhold and ESC [ ... CSI (and every
// other escape) reaches the emulator intact.
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

// stripFromOSCCode reads the numeric OSC code that follows ESC ]. OSC 0/1/2 are single-digit,
// so a SECOND digit proves the code is >= 10 (a colour/hyperlink/etc. OSC) and the whole
// sequence is flushed through. On the first non-digit byte the accumulated code decides: a
// title/icon code (0/1/2) switches to discard, any other (including an empty code) flushes the
// withheld introducer and passes the rest through.
func (m *vtModel) stripFromOSCCode(
	out []byte,
	c byte,
) []byte {
	if c >= '0' && c <= '9' {
		m.stripWithheld = append(m.stripWithheld, c)
		if len(m.stripCode) == 0 {
			m.stripCode = append(m.stripCode, c)
			return out
		}
		// Second digit: code >= 10, never OSC 0/1/2 — keep the whole OSC for x/vt.
		m.stripCode = append(m.stripCode, c)
		out = m.flushWithheld(out)
		m.stripState = stripOSCPass
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

// oscCodeIsTitle reports whether the accumulated code is an icon/title OSC (0, 1 or 2). The
// exact single-digit match mirrors escan's capture set precisely: OSC 10/11/12 (code "10"…)
// and an empty code are NOT titles, so this machine and escan always agree on which OSCs are
// captured-and-stripped versus passed through.
func (m *vtModel) oscCodeIsTitle() bool {
	switch string(m.stripCode) {
	case "0", "1", "2":
		return true
	default:
		return false
	}
}

// stripFromDiscard drops the body of an OSC 0/1/2 until a UTF-8-safe terminator: BEL commits,
// ESC may begin a 7-bit ST (deferred to stripFromDiscardEsc), CAN/SUB abort. Every other byte
// — crucially INCLUDING the 8-bit ST 0x9C and all bytes >= 0x80 — is title content that is
// dropped, never a terminator. Nothing is emitted.
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
