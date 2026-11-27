package model

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

const maxOSCTextRunes = 256

const decstr = "\x1b[!p"

const deckpam = "\x1b="

var serializedModeOrder = []int{1, 1000, 1002, 1003, 1006, 1015, 1004, 2004}

type vtSerializer struct{}

var _ Serializer = vtSerializer{}

func (vtSerializer) Serialize(
	m TerminalModel,
) []byte {
	vm := m.(*vtModel)
	var b strings.Builder
	alt := vm.emu.IsAltScreen()
	vm.reconcileAltScreen(alt)
	sh := &vm.shadow
	cols, rows := vm.emu.Width(), vm.emu.Height()

	writePreamble(&b, sh, alt)
	writeContent(&b, vm, alt, cols, rows)
	writeModes(&b, sh, rows)
	writeCursor(&b, vm, sh)
	writeChrome(&b, sh)

	b.WriteString(ansi.ResetStyle)
	return []byte(b.String())
}

func writePreamble(
	b *strings.Builder,
	sh *shadowState,
	alt bool,
) {
	b.WriteString(decstr)
	b.WriteString(ansi.SetMode(ansi.DECMode(7)))
	if sh.fgSet {
		b.WriteString(oscColor(10, sh.fg))
	}
	if sh.bgSet {
		b.WriteString(oscColor(11, sh.bg))
	}
	if sh.cursorColorSet {
		b.WriteString(oscColor(12, sh.cursorColor))
	}
	if alt {
		b.WriteString(ansi.SetMode(ansi.DECMode(1049)))
	}
	b.WriteString(ansi.EraseDisplay(2))
	b.WriteString(ansi.CursorPosition(1, 1))
}

func writeContent(
	b *strings.Builder,
	vm *vtModel,
	alt bool,
	cols int,
	rows int,
) {
	emu := vm.emu
	var line []byte
	if !alt {
		n := emu.ScrollbackLen()
		for i := 0; i < n; i++ {
			next := chainNextOf(emu.ScrollbackRowWrapped(i+1), emu.ScrollbackLine(i+1).At(0))
			if i == n-1 {
				next = chainNextOf(emu.RowWrapped(0), emu.CellAt(0, 0))
			}
			line = appendChainRow(line[:0], emu.ScrollbackLine(i), cols, emu.ScrollbackRowWrapped(i), next, "\r\n")
			b.Write(line)
		}
	}
	var scratch []uv.Cell
	for y := 0; y < rows; y++ {
		brk := "\r\n"
		if y == rows-1 {
			brk = ""
		}
		scratch = snapshotRowInto(emu, scratch, cols, y)
		next := chainNextOf(emu.RowWrapped(y+1), emu.CellAt(0, y+1))
		line = appendChainRow(line[:0], scratch, cols, emu.RowWrapped(y), next, brk)
		b.Write(line)
	}
}

func writeModes(
	b *strings.Builder,
	sh *shadowState,
	rows int,
) {
	for _, mode := range serializedModeOrder {
		if sh.modes[mode] {
			b.WriteString(ansi.SetMode(ansi.DECMode(mode)))
		}
	}
	if sh.keypadApplication {
		b.WriteString(deckpam)
	}
	if on, ok := sh.modes[7]; ok && !on {
		b.WriteString(ansi.ResetMode(ansi.DECMode(7)))
	}
	writeCharset(b, sh)
	if sh.scrollRegionSet && (sh.scrollTop != 1 || sh.scrollBottom != rows) {
		b.WriteString(ansi.SetTopBottomMargins(sh.scrollTop, sh.scrollBottom))
	}
}

func writeCursor(
	b *strings.Builder,
	vm *vtModel,
	sh *shadowState,
) {
	origin := sh.modes[6]
	if origin {
		b.WriteString(ansi.SetMode(ansi.DECMode(6)))
	}
	pos := vm.emu.CursorPosition()
	row, col := pos.Y+1, pos.X+1
	if origin {
		top := 1
		if sh.scrollRegionSet {
			top = sh.scrollTop
		}
		row -= top - 1
	}
	b.WriteString(ansi.CursorPosition(col, row))
}

func writeChrome(
	b *strings.Builder,
	sh *shadowState,
) {
	if sh.cursorShapeSet {
		b.WriteString(ansi.SetCursorStyle(decscusr(sh.cursorShape, sh.cursorBlink)))
	}
	if sh.cursorVisible {
		b.WriteString(ansi.SetMode(ansi.DECMode(25)))
	} else {
		b.WriteString(ansi.ResetMode(ansi.DECMode(25)))
	}
	if icon := sanitizeOSCText(sh.iconName); icon != "" {
		b.WriteString(oscTitleSeq(1, icon))
	}
	if title := sanitizeOSCText(sh.title); title != "" {
		b.WriteString(oscTitleSeq(2, title))
	}
}

func oscTitleSeq(
	code int,
	sanitized string,
) string {
	return fmt.Sprintf("\x1b]%d;%s\x1b\\", code, sanitized)
}

func writeCharset(
	b *strings.Builder,
	sh *shadowState,
) {
	g1set := sh.g1 != 'B'
	if sh.g0 != 'B' {
		b.WriteString("\x1b(" + string(rune(sh.g0)))
	}
	if g1set {
		b.WriteString("\x1b)" + string(rune(sh.g1)))
	}
	switch {
	case sh.glLock == 1:
		b.WriteByte(0x0e)
	case g1set:
		b.WriteByte(0x0f)
	}
}

func encodeLine(
	cells []uv.Cell,
	width int,
	trim bool,
) string {
	line, _ := appendLine(nil, cells, width, trim)
	return string(line)
}

func appendLine(
	dst []byte,
	cells []uv.Cell,
	width int,
	trim bool,
) ([]byte, int) {
	if width > len(cells) {
		width = len(cells)
	}
	last := width
	if trim {
		last = 0
		for i := 0; i < width; i++ {
			if !cellBlank(&cells[i]) {
				last = i + 1
			}
		}
	}
	var cur uv.Style
	i := 0
	for ; i < last; i++ {
		c := &cells[i]
		dst = append(dst, uv.StyleDiff(&cur, &c.Style)...)
		cur = c.Style
		if c.Content == "" {
			dst = append(dst, ' ')
		} else {
			dst = append(dst, c.Content...)
		}
		if c.Width > 1 {
			i += c.Width - 1
		}
	}
	if !cur.IsZero() {
		dst = append(dst, ansi.ResetStyle...)
	}
	return dst, i
}

func cellBlank(
	c *uv.Cell,
) bool {
	if !c.Style.IsZero() {
		return false
	}
	if !c.Link.IsZero() {
		return false
	}
	return c.Content == "" || c.Content == " "
}

func oscColor(
	code int,
	c color.Color,
) string {
	return fmt.Sprintf("\x1b]%d;%s\x1b\\", code, formatColor(c))
}

func formatColor(
	c color.Color,
) string {
	r, g, bl, _ := c.RGBA()
	return fmt.Sprintf("rgb:%04x/%04x/%04x", r, g, bl)
}

func decscusr(
	shape vt.CursorStyle,
	blink bool,
) int {
	base := 1
	switch shape { //nolint:exhaustive // CursorBlock is the base=1 initializer default; only underline/bar override it.
	case vt.CursorUnderline:
		base = 3
	case vt.CursorBar:
		base = 5
	}
	if !blink {
		base++
	}
	return base
}

func sanitizeOSCText(
	s string,
) string {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		if r >= 0x80 && r <= 0x9f {
			continue
		}
		out = append(out, r)
		if len(out) >= maxOSCTextRunes {
			break
		}
	}
	return string(out)
}
