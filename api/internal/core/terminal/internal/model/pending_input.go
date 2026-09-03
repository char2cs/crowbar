package model

const (
	framingGround = iota
	framingEsc
	framingEscIntermediate
	framingCSI
	framingOSC
	framingOSCEsc
	framingString
	framingStringEsc
)

// scanPartial runs a minimal ECMA-48 escape-framing state machine over b and returns the
// trailing bytes of an incomplete escape/control sequence, or nil when b ends in ground
// state. It recognises sequence FRAMING only (never semantics): ground, ESC, two-byte
// ESC + intermediate, CSI (ESC [ ... final 0x40-0x7E), OSC (ESC ] ... BEL or 7-bit ST
// ESC \), and the DCS/SOS/PM/APC string family (ESC P/X/^/_ ... 7-bit ST), with CAN (0x18)
// and SUB (0x1A) aborting to ground.
//
// UTF-8 TRANSPARENCY (the load-bearing property). The emulator this scanner shadows is fed
// a UTF-8 byte stream, and a UTF-8 terminal does NOT decode the 8-bit C1 range (0x80-0x9F)
// as control introducers: those bytes only ever appear as the lead or continuation bytes of
// a multi-byte UTF-8 character (e.g. "▘" U+2598 = E2 96 98, whose 0x98 byte is the C1 SOS
// code point). Treating them as C1 introducers (or as an 8-bit ST 0x9C terminator inside a
// string) is exactly the defect this scanner previously had: a single box-drawing glyph mid
// frame flipped it into a string/CSI state that then swallowed the whole rest of the frame,
// so PendingInput() returned a hundreds-of-bytes, PRINTABLE-leading blob instead of nil.
// Attach appends that after the clean serialize and paints app chrome into the input line.
// The scanner is therefore restricted to the 7-bit C0/ESC framing the emulator actually
// honours; every byte >= 0x80 is inert ground content. The one and only sequence introducer
// is the 7-bit ESC (0x1B), so a non-nil return ALWAYS starts with ESC.
func scanPartial(
	b []byte,
) []byte {
	state := framingGround
	start := 0
	for i := 0; i < len(b); i++ {
		state, start = framingStep(state, b[i], i, start)
	}
	if state == framingGround {
		return nil
	}
	return b[start:]
}

func framingStep(
	state int,
	c byte,
	i int,
	start int,
) (int, int) {
	switch state {
	case framingGround:
		return framingFromGround(c, i, start)
	case framingEsc:
		return framingFromEsc(c), start
	case framingEscIntermediate:
		return framingFromEscIntermediate(c), start
	case framingCSI:
		return framingFromCSI(c), start
	case framingOSC:
		return framingFromOSC(c), start
	case framingOSCEsc:
		return framingFromStringEsc(c, framingOSC), start
	case framingString:
		return framingFromString(c), start
	default:
		return framingFromStringEsc(c, framingString), start
	}
}

func framingFromGround(
	c byte,
	i int,
	start int,
) (int, int) {
	// Only the 7-bit ESC introduces a sequence. Bytes >= 0x80 are UTF-8 lead/continuation
	// bytes in this stream, never 8-bit C1 introducers (see scanPartial's UTF-8 note), so
	// they remain inert ground content.
	if c == 0x1b {
		return framingEsc, i
	}
	return framingGround, start
}

func framingFromEsc(
	c byte,
) int {
	switch {
	case c == '[':
		return framingCSI
	case c == ']':
		return framingOSC
	case c == 'P' || c == 'X' || c == '^' || c == '_':
		return framingString
	case c == 0x18 || c == 0x1a:
		return framingGround
	case c >= 0x20 && c <= 0x2f:
		return framingEscIntermediate
	default:
		return framingGround
	}
}

func framingFromEscIntermediate(
	c byte,
) int {
	if c >= 0x20 && c <= 0x2f {
		return framingEscIntermediate
	}
	return framingGround
}

func framingFromCSI(
	c byte,
) int {
	switch {
	case c == 0x18 || c == 0x1a:
		return framingGround
	case c >= 0x40 && c <= 0x7e:
		return framingGround
	default:
		return framingCSI
	}
}

func framingFromOSC(
	c byte,
) int {
	// BEL (7-bit) and ESC \ terminate; the 8-bit ST 0x9C is NOT honoured because a UTF-8
	// byte inside the OSC string (e.g. a title glyph) can equal 0x9C and must not close it.
	switch c {
	case 0x07:
		return framingGround
	case 0x1b:
		return framingOSCEsc
	case 0x18, 0x1a:
		return framingGround
	default:
		return framingOSC
	}
}

func framingFromString(
	c byte,
) int {
	// Only ESC \ (7-bit ST) and CAN/SUB terminate; the 8-bit ST 0x9C is NOT honoured, for
	// the same UTF-8-transparency reason as framingFromOSC.
	switch c {
	case 0x1b:
		return framingStringEsc
	case 0x18, 0x1a:
		return framingGround
	default:
		return framingString
	}
}

func framingFromStringEsc(
	c byte,
	back int,
) int {
	if c == '\\' {
		return framingGround
	}
	return back
}
