package vt

import (
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// handlePrint handles printable characters.
func (e *Emulator) handlePrint(r rune) {
	if r >= ansi.SP && r < ansi.DEL {
		if len(e.grapheme) > 0 {
			// If we have a grapheme buffer, flush it before handling the ASCII character.
			e.flushGrapheme()
		}
		e.handleGrapheme(string(r), 1)
	} else {
		e.grapheme = append(e.grapheme, r)
	}
}

// flushGrapheme flushes the current grapheme buffer, if any, and handles the
// grapheme as a single unit.
func (e *Emulator) flushGrapheme() {
	if len(e.grapheme) == 0 {
		return
	}

	// XXX: We always use [ansi.GraphemeWidth] here to report accurate widths
	// and it's up to the caller to decide how to handle Unicode vs non-Unicode
	// modes.
	method := ansi.GraphemeWidth
	graphemes := string(e.grapheme)
	for len(graphemes) > 0 {
		cluster, width := ansi.FirstGraphemeCluster(graphemes, method)
		e.handleGrapheme(cluster, width)
		graphemes = graphemes[len(cluster):]
	}
	e.grapheme = e.grapheme[:0] // Reset the grapheme buffer.
}

// handleGrapheme handles UTF-8 graphemes.
func (e *Emulator) handleGrapheme(content string, width int) {
	awm := e.isModeSet(ansi.ModeAutoWrap)
	cell := uv.Cell{
		Content: content,
		Width:   width,
		Style:   e.scr.cursorPen(),
		Link:    e.scr.cursorLink(),
	}

	x, y := e.scr.CursorPosition()
	if e.atPhantom && awm {
		// moves cursor down similar to [Terminal.linefeed] except it doesn't
		// respects [ansi.LNM] mode.
		// This will reset the phantom state i.e. pending wrap state.
		e.index()
		_, y = e.scr.CursorPosition()
		x = 0
		e.scr.setWrapped(y, true) // the new row continues the one that overflowed
	}

	// Handle character set mappings
	if len(content) == 1 { //nolint:nestif
		var charset CharSet
		c := content[0]
		if e.gsingle > 1 && e.gsingle < 4 {
			charset = e.charsets[e.gsingle]
			e.gsingle = 0
		} else if c < 128 {
			charset = e.charsets[e.gl]
		} else {
			charset = e.charsets[e.gr]
		}

		if charset != nil {
			if r, ok := charset[c]; ok {
				cell.Content = r
				cell.Width = 1
			}
		}
	}

	if cell.Width == 1 && len(content) == 1 {
		e.lastChar, _ = utf8.DecodeRuneInString(content)
	}

	cols := e.scr.Width()
	x, y = e.wrapEarly(awm, x, y, &cell)
	e.scr.SetCell(x, y, &cell)

	// A glyph reaching the last column leaves a pending wrap, as in xterm.js; the cursor rests
	// on the last column.
	e.atPhantom = awm && x+cell.Width >= cols
	if e.atPhantom {
		x = cols - 1
	} else {
		x += cell.Width
	}

	// NOTE: We don't reset the phantom state here, we handle it up above.
	e.scr.setCursor(x, y, false)
}

// wrapEarly moves a glyph that does not fit the rest of row y to the start of the next row, as
// xterm.js does (InputHandler.print): the cells it skips are blanked in its pen, and the new row
// continues row y. It returns where the glyph goes.
func (e *Emulator) wrapEarly(awm bool, x, y int, cell *uv.Cell) (int, int) {
	if !awm || x == 0 || x+cell.Width <= e.scr.Width() {
		return x, y
	}
	blank := uv.Cell{Content: " ", Width: 1, Style: cell.Style, Link: cell.Link}
	for ; x < e.scr.Width(); x++ {
		e.scr.SetCell(x, y, &blank)
	}
	e.index()
	_, y = e.scr.CursorPosition()
	e.scr.setWrapped(y, true)
	return 0, y
}
