// Link detection and activation for terminal output (URLs and file references).
// Rows are joined into logical lines before matching, because the daemon repaints
// row by row and xterm never learns a row wrapped (see selection-text.ts): a
// per-row match would crop a long link at the pane width.
import type { IBufferRange, IDisposable, ILink, Terminal } from '@xterm/xterm'
import { IS_MAC } from '@/utils/platform'
import { parseFileLinkCandidates, resolveFileLinkPath } from './terminal-file-links'
import { rowRunsToTheEdge, type SelectionSource } from '../utils/selection-text'

export interface TerminalLinksOptions {
  /** The directory relative paths resolve against (terminal cwd / worktree). */
  getRoot: () => string | undefined
  /** Opens the resolved absolute path in Crowbar's editor. */
  openFile: (absolutePath: string, line?: number) => void
  /** Called when an activated file candidate cannot be resolved to an absolute path. */
  onUnresolved?: (candidateText: string) => void
  /** Called when a URL link is activated, with the pointer position. */
  onUrl: (url: string, position: { x: number; y: number }) => void
}

// The same pattern @xterm/addon-web-links uses, so trailing punctuation is
// trimmed identically.
const URL_RE = /(https?|HTTP?):\/\/[^\s"'!*(){}|\\^<>`]*[^\s"':,.!?{}|\\^~[\]`()<>]/g

// Bounds a pathological run of full-width rows (e.g. `yes | tr -d '\n'`).
const MAX_LOGICAL_ROWS = 64

interface LogicalLine {
  text: string
  /** 0-based buffer row of each segment and the string offset it starts at. */
  rows: Array<{ y: number; start: number }>
}

function continuesOnNextRow(source: SelectionSource, y: number): boolean {
  const next = source.getLine(y + 1)
  return !!next && (next.isWrapped || rowRunsToTheEdge(source, y))
}

function readLogicalLine(source: SelectionSource, y: number): LogicalLine {
  let top = y
  while (top > 0 && y - top < MAX_LOGICAL_ROWS && continuesOnNextRow(source, top - 1)) top--
  let bottom = y
  while (bottom - y < MAX_LOGICAL_ROWS && continuesOnNextRow(source, bottom)) bottom++

  const rows: LogicalLine['rows'] = []
  let text = ''
  for (let row = top; row <= bottom; row++) {
    rows.push({ y: row, start: text.length })
    // A row that runs on is read at full width, so offsets map 1:1 to columns.
    text += source.getLine(row)?.translateToString(row === bottom) ?? ''
  }
  return { text, rows }
}

function positionAt(line: LogicalLine, offset: number): { x: number; y: number } {
  let segment = line.rows[0]
  for (const row of line.rows) {
    if (row.start > offset) break
    segment = row
  }
  return { x: offset - segment.start, y: segment.y }
}

/** 1-based inclusive xterm range for the [start, end) string offsets. */
function rangeOf(line: LogicalLine, start: number, end: number): IBufferRange {
  const from = positionAt(line, start)
  const to = positionAt(line, end - 1)
  return {
    start: { x: from.x + 1, y: from.y + 1 },
    end: { x: to.x + 1, y: to.y + 1 },
  }
}

interface FoundLink {
  text: string
  kind: 'url' | 'file'
  start: number
  path?: string
  line?: number
}

function findLinks(text: string): FoundLink[] {
  const found: FoundLink[] = []
  for (const m of text.matchAll(URL_RE)) {
    if (URL.canParse(m[0])) found.push({ text: m[0], kind: 'url', start: m.index })
  }
  for (const c of parseFileLinkCandidates(text)) {
    found.push({ text: c.text, kind: 'file', start: c.startIndex, path: c.path, line: c.line })
  }
  return found
}

/** Cmd on macOS, Ctrl elsewhere. */
function isPrimaryModifier(event: { metaKey: boolean; ctrlKey: boolean }): boolean {
  return IS_MAC ? event.metaKey : event.ctrlKey
}

// Registers the one link provider for a terminal, and the handler for OSC 8
// hyperlinks (escape-sequence links xterm detects itself). Links activate only
// with the primary modifier held; for provider links the underline and pointer
// cursor follow the same gate (xterm always decorates OSC 8 links).
export function registerTerminalLinks(
  terminal: Terminal,
  options: TerminalLinksOptions,
): IDisposable {
  let modifierHeld = false
  let hovered: ILink | null = null
  const setModifierHeld = (held: boolean) => {
    modifierHeld = held
    if (!hovered?.decorations) return
    hovered.decorations.underline = held
    hovered.decorations.pointerCursor = held
  }
  const onModifierEvent = (event: KeyboardEvent | MouseEvent) =>
    setModifierHeld(isPrimaryModifier(event))
  const onBlur = () => setModifierHeld(false)
  const element = terminal.element
  element?.addEventListener('keydown', onModifierEvent)
  element?.addEventListener('keyup', onModifierEvent)
  element?.addEventListener('mousemove', onModifierEvent)
  window.addEventListener('blur', onBlur)

  const activateUrl = (url: string, event: MouseEvent) =>
    options.onUrl(url, { x: event.clientX, y: event.clientY })

  const activate = (found: FoundLink, event: MouseEvent) => {
    if (!isPrimaryModifier(event)) return
    if (found.kind === 'url') {
      activateUrl(found.text, event)
      return
    }
    const resolved = resolveFileLinkPath(found.path ?? found.text, options.getRoot())
    if (resolved) options.openFile(resolved, found.line)
    else options.onUnresolved?.(found.text)
  }

  // Without a handler xterm activates OSC 8 links on a plain click.
  terminal.options.linkHandler = {
    activate: (event, url) => {
      if (isPrimaryModifier(event)) activateUrl(url, event)
    },
  }

  const provider = terminal.registerLinkProvider({
    provideLinks(bufferLineNumber: number, callback: (links: ILink[] | undefined) => void) {
      const y = bufferLineNumber - 1
      const source: SelectionSource = {
        cols: terminal.cols,
        getLine: (row) => terminal.buffer.active.getLine(row),
      }
      if (!source.getLine(y)) {
        callback(undefined)
        return
      }
      const line = readLogicalLine(source, y)
      const links: ILink[] = []
      for (const found of findLinks(line.text)) {
        const range = rangeOf(line, found.start, found.start + found.text.length)
        if (range.start.y > bufferLineNumber || range.end.y < bufferLineNumber) continue
        const link: ILink = {
          text: found.text,
          range,
          decorations: { underline: modifierHeld, pointerCursor: modifierHeld },
          activate: (event) => activate(found, event),
          hover: (event) => {
            hovered = link
            setModifierHeld(isPrimaryModifier(event))
          },
          leave: () => {
            if (hovered === link) hovered = null
          },
        }
        links.push(link)
      }
      callback(links.length > 0 ? links : undefined)
    },
  })

  return {
    dispose() {
      provider.dispose()
      terminal.options.linkHandler = null
      element?.removeEventListener('keydown', onModifierEvent)
      element?.removeEventListener('keyup', onModifierEvent)
      element?.removeEventListener('mousemove', onModifierEvent)
      window.removeEventListener('blur', onBlur)
    },
  }
}
