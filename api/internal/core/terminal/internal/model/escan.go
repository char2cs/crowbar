package model

import (
	"bytes"
	"strconv"
	"strings"
)

const maxEscanParams = 64

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

func (m *vtModel) scanCharsetAndRegion(
	p []byte,
) {
	for _, c := range p {
		m.escanStep(c)
	}
}

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
	default:
		m.escanFromOSCEsc(c)
	}
}

func (m *vtModel) escanFromGround(
	c byte,
) {
	switch c {
	case 0x0e:
		m.shadow.glLock = 1
	case 0x0f:
		m.shadow.glLock = 0
	case 0x1b:
		m.escanState = escEsc
	}
}

func (m *vtModel) escanFromEsc(
	c byte,
) {
	switch c {
	case '(':
		m.escanState = escSCSG0
	case ')':
		m.escanState = escSCSG1
	case '[':
		m.beginCSI()
	case ']':
		m.beginOSC()
	case 'c':
		m.applyRIS()
		m.escanState = escGround
	default:
		m.escanState = escGround
	}
}

func (m *vtModel) applyRIS() {
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
	m.shadow.g0, m.shadow.g1, m.shadow.glLock = 'B', 'B', 0
}

func (m *vtModel) escanFromSCS(
	c byte,
	slot int,
) {
	if c >= 0x20 && c <= 0x2f {
		return
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
	case c == 0x18 || c == 0x1a:
		m.escanState = escGround
	case c == 0x1b:
		m.escanState = escEsc
	case c >= 0x40 && c <= 0x7e:
		m.finishCSI(c)
		m.escanState = escGround
	case c >= 0x3c && c <= 0x3f:
		m.escanPrivate = true
		m.appendEscanParam(c)
	default:
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

func (m *vtModel) finishCSI(
	final byte,
) {
	if final != 'r' || m.escanPrivate {
		return
	}
	m.applyDECSTBM(string(m.escanParams))
}

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
			return 0, 0, false
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
		return 0, 0, false
	}
	return top, bottom, true
}

func (m *vtModel) beginOSC() {
	m.escanState = escOSC
	m.escanOSC = m.escanOSC[:0]
}

func (m *vtModel) escanFromOSC(
	c byte,
) {
	switch c {
	case 0x07:
		m.finishOSC()
		m.escanState = escGround
	case 0x1b:
		m.escanState = escOSCEsc
	case 0x18, 0x1a:
		m.escanState = escGround
	default:
		if len(m.escanOSC) < maxEscanOSCText {
			m.escanOSC = append(m.escanOSC, c)
		}
	}
}

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
