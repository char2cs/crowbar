package model

import "strconv"

const (
	stripGround = iota
	stripEsc
	stripOSCCode
	stripOSCDiscard
	stripOSCDiscardEsc
	stripOSCPass
	stripOSCPassEsc
)

func (m *vtModel) stripOSCTitles(
	p []byte,
) []byte {
	out := make([]byte, 0, len(p))
	for _, c := range p {
		out = m.stripStep(out, c)
	}
	return out
}

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
	default:
		return m.stripFromPassEsc(out, c)
	}
}

func (m *vtModel) stripFromGround(
	out []byte,
	c byte,
) []byte {
	if c == 0x1b {
		m.stripWithheld = append(m.stripWithheld[:0], c)
		m.stripState = stripEsc
		return out
	}
	return append(out, c)
}

func (m *vtModel) stripFromEsc(
	out []byte,
	c byte,
) []byte {
	if c == ']' {
		m.stripWithheld = append(m.stripWithheld, c)
		m.stripCode = m.stripCode[:0]
		m.stripState = stripOSCCode
		return out
	}
	out = m.flushWithheld(out)
	m.stripState = stripGround
	return m.stripStep(out, c)
}

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

func (m *vtModel) oscCodeIsTitle() bool {
	code, err := strconv.Atoi(string(m.stripCode))
	if err != nil {
		return false
	}
	return code == 0 || code == 1 || code == 2
}

func (m *vtModel) stripFromDiscard(
	out []byte,
	c byte,
) []byte {
	switch c {
	case 0x07:
		m.stripState = stripGround
	case 0x1b:
		m.stripState = stripOSCDiscardEsc
	case 0x18, 0x1a:
		m.stripState = stripGround
	}
	return out
}

func (m *vtModel) stripFromDiscardEsc(
	out []byte,
	c byte,
) []byte {
	if c == '\\' {
		m.stripState = stripGround
		return out
	}
	m.stripState = stripOSCDiscard
	return m.stripFromDiscard(out, c)
}

func (m *vtModel) stripFromPass(
	out []byte,
	c byte,
) []byte {
	out = append(out, c)
	switch c {
	case 0x07, 0x18, 0x1a:
		m.stripState = stripGround
	case 0x1b:
		m.stripState = stripOSCPassEsc
	}
	return out
}

func (m *vtModel) stripFromPassEsc(
	out []byte,
	c byte,
) []byte {
	if c == '\\' {
		out = append(out, c)
		m.stripState = stripGround
		return out
	}
	m.stripState = stripOSCPass
	return m.stripFromPass(out, c)
}

func (m *vtModel) flushWithheld(
	out []byte,
) []byte {
	out = append(out, m.stripWithheld...)
	m.stripWithheld = m.stripWithheld[:0]
	return out
}
