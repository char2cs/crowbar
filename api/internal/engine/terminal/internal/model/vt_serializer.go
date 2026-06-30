package model

import (
	"fmt"
	"image/color"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

const maxOSCTextRunes = 256

// decstr is the DECSTR (soft terminal reset) sequence. x/ansi has no constant for it.
const decstr = "\x1b[!p"

// serializedModeOrder is the deterministic emission order for the default-OFF DEC private
// modes re-asserted in step 8 (DECCKM, the three mouse-tracking modes, the two
// mouse-encoding modes, focus reporting, and bracketed paste).
var serializedModeOrder = []int{1, 1000, 1002, 1003, 1006, 1015, 1004, 2004}

// vtSerializer turns a paired *vtModel into one clean ground-state ANSI redraw. It is
// stateless, so it is a zero-field value type rather than a constructed struct.
type vtSerializer struct{}

var _ Serializer = vtSerializer{}

// Serialize recovers its paired concrete model via a type assertion (constructed together
// by New, so the assertion cannot fail in production) and emits the redraw bytes in the
// fixed 17-step order.
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

// writePreamble emits steps 1-4: soft reset + forced autowrap-ON, app-set default colours,
// alt-screen enter, and clear+home.
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

// writeContent emits steps 5-7: the scrollback flow (primary buffer only) followed
// immediately by exactly rows visible grid rows, as one continuous stream so the trailing
// rows physical lines are the visible viewport and the earlier lines land in history.
func writeContent(
	b *strings.Builder,
	vm *vtModel,
	alt bool,
	cols int,
	rows int,
) {
	if !alt {
		n := vm.emu.ScrollbackLen()
		for y := 0; y < n; y++ {
			line := vm.emu.ScrollbackLine(y)
			b.WriteString(encodeLine(line, len(line), true))
			b.WriteString("\r\n")
		}
	}
	for y := 0; y < rows; y++ {
		if y > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(encodeGridRow(vm.emu, cols, y))
	}
}

// writeModes emits steps 8-11: the default-OFF private modes, the autowrap reset (only
// when the app disabled it), the charset designation + active locking shift, and the
// scroll region (only when set and not full-screen).
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
	if on, ok := sh.modes[7]; ok && !on {
		b.WriteString(ansi.ResetMode(ansi.DECMode(7)))
	}
	writeCharset(b, sh)
	if sh.scrollRegionSet && !(sh.scrollTop == 1 && sh.scrollBottom == rows) {
		b.WriteString(ansi.SetTopBottomMargins(sh.scrollTop, sh.scrollBottom))
	}
}

// writeCursor emits steps 12-13: origin mode (before the cursor move so the final CUP is
// interpreted in the region's coordinate space) and the cursor position itself, in
// region-relative coordinates when origin mode is on.
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

// writeChrome emits steps 14-16: cursor shape (only when the app explicitly set one),
// cursor visibility (always, one deterministic byte), and the sanitized title/icon.
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
		b.WriteString("\x1b]1;" + icon + "\x1b\\")
	}
	if title := sanitizeOSCText(sh.title); title != "" {
		b.WriteString("\x1b]2;" + title + "\x1b\\")
	}
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

func encodeGridRow(
	emu emulator,
	cols int,
	y int,
) string {
	cells := make([]uv.Cell, cols)
	for x := 0; x < cols; x++ {
		c := emu.CellAt(x, y)
		if c == nil {
			cells[x] = uv.EmptyCell
			continue
		}
		cells[x] = *c
	}
	return encodeLine(cells, cols, true)
}

// encodeLine renders cells (up to width columns) into SGR-run-length ANSI, trimming
// trailing blank cells when trim is set and resetting the pen at the row's end so style
// never bleeds across the following line break.
func encodeLine(
	cells []uv.Cell,
	width int,
	trim bool,
) string {
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
	var b strings.Builder
	var cur uv.Style
	for i := 0; i < last; i++ {
		c := cells[i]
		b.WriteString(uv.StyleDiff(&cur, &c.Style))
		cur = c.Style
		content := c.Content
		if content == "" {
			content = " "
		}
		b.WriteString(content)
		if c.Width > 1 {
			i += c.Width - 1
		}
	}
	if !cur.IsZero() {
		b.WriteString(ansi.ResetStyle)
	}
	return b.String()
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

// oscColor builds an OSC 10/11/12 default-colour sequence in the canonical, ST-terminated
// xterm 16-bit form (rgb:RRRR/GGGG/BBBB), never a bare BEL, so it round-trips losslessly.
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

// decscusr maps a vt.CursorStyle plus blink flag to its DECSCUSR parameter: block 1/2,
// underline 3/4, bar 5/6, with the odd member blinking and the even member steady.
func decscusr(
	shape vt.CursorStyle,
	blink bool,
) int {
	base := 1
	switch shape {
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

// sanitizeOSCText makes app-controlled OSC string text safe to re-emit inside our own
// OSC 1/2 ... ST envelope: invalid UTF-8 becomes U+FFFD, every C0 control (including ESC
// and BEL), DEL, and every C1 control (including the 8-bit ST) is dropped, and the result
// is capped at maxOSCTextRunes runes on a rune boundary.
func sanitizeOSCText(
	s string,
) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
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
