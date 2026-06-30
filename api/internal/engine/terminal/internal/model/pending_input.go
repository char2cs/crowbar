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
// ESC + intermediate, CSI (ESC [ ... final 0x40-0x7E), OSC (ESC ] ... ST or BEL), and the
// DCS/SOS/PM/APC string family (ESC P/X/^/_ ... ST), plus the 8-bit C1 introducers, with
// CAN (0x18) and SUB (0x1A) aborting to ground.
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
	switch {
	case c == 0x1b:
		return framingEsc, i
	case c == 0x9b:
		return framingCSI, i
	case c == 0x9d:
		return framingOSC, i
	case c == 0x90 || c == 0x98 || c == 0x9e || c == 0x9f:
		return framingString, i
	default:
		return framingGround, start
	}
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
	switch {
	case c == 0x07 || c == 0x9c:
		return framingGround
	case c == 0x1b:
		return framingOSCEsc
	case c == 0x18 || c == 0x1a:
		return framingGround
	default:
		return framingOSC
	}
}

func framingFromString(
	c byte,
) int {
	switch {
	case c == 0x9c:
		return framingGround
	case c == 0x1b:
		return framingStringEsc
	case c == 0x18 || c == 0x1a:
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
