// Reads a terminal selection back as text WITHOUT the line breaks the daemon's
// renderer would otherwise fake.
//
// Why this exists at all: the daemon owns the VT screen model and repaints the
// client by addressing every changed row absolutely — `CUP(y,1)` + erase +
// the row's cells (api/.../model/diff.go, writeScreenDiff). The client's xterm
// therefore never AUTO-WRAPS: a 300-column line printed by the shell arrives as
// two independent row writes, and xterm sets `isWrapped` only on a row the
// cursor wrapped onto by itself. It stays false on every row the daemon paints.
//
// xterm's own `getSelection()` uses that flag to decide where the newlines go,
// so with the flag permanently false it inserts one at EVERY row boundary. Copy
// a long path, URL, or command out of the terminal and it comes back snapped in
// half at the terminal width — the "copied text is broken" bug.
//
// The fix is to answer the same question the flag was meant to answer from the
// only evidence the client still has: a row whose LAST column holds a written
// cell had nowhere left to put the next character, so the row below it is its
// continuation. Rows that ended early leave their trailing cells untouched, and
// an untouched cell reads back as '' — not ' ' — which is what separates the two
// cases cleanly. The flag is still honoured when it IS set (the snapshot path
// replays scrollback as whole logical lines, which really do auto-wrap).
//
// The one ambiguity this cannot resolve is a line that happens to end exactly at
// the last column: it is indistinguishable from a wrap. Every terminal that has
// lost its wrap metadata has that same ambiguity, and joining is the reading
// that keeps pasted commands runnable.

/** The subset of xterm's `IBufferLine` this needs. */
export interface SelectionLine {
  isWrapped: boolean
  translateToString(trimRight?: boolean, startColumn?: number, endColumn?: number): string
  getCell(x: number): { getChars(): string } | undefined
}

/** The subset of xterm's `IBuffer` (plus the terminal's width) this needs. */
export interface SelectionSource {
  cols: number
  getLine(y: number): SelectionLine | undefined
}

/** A buffer range, matching xterm's `getSelectionPosition()`. `end.x` is exclusive. */
export interface SelectionRange {
  start: { x: number; y: number }
  end: { x: number; y: number }
}

const NON_BREAKING_SPACE = /\u00A0/g

/**
 * True when row `y` is filled all the way to its last column, i.e. the text
 * continues on the row below.
 */
function rowRunsToTheEdge(source: SelectionSource, y: number): boolean {
  const cell = source.getLine(y)?.getCell(source.cols - 1)
  return !!cell && cell.getChars() !== ''
}

function order(range: SelectionRange): SelectionRange {
  const { start, end } = range
  const reversed = end.y < start.y || (end.y === start.y && end.x < start.x)
  return reversed ? { start: end, end: start } : range
}

export function selectionTextPreservingWraps(
  source: SelectionSource,
  range: SelectionRange,
): string {
  const { start, end } = order(range)
  if (start.x === end.x && start.y === end.y) return ''

  const first = source.getLine(start.y)
  if (!first) return ''

  // Mirrors xterm's own slicing: the first row stops at the selection end only
  // when the whole selection sits on one row.
  const lines = [first.translateToString(true, start.x, start.y === end.y ? end.x : undefined)]

  for (let y = start.y + 1; y <= end.y; y++) {
    const line = source.getLine(y)
    if (!line) continue
    const text = y === end.y ? line.translateToString(true, 0, end.x) : line.translateToString(true)
    if (line.isWrapped || rowRunsToTheEdge(source, y - 1)) lines[lines.length - 1] += text
    else lines.push(text)
  }

  return lines.join('\n').replace(NON_BREAKING_SPACE, ' ')
}
