// Links in terminal output: URLs open in the browser, path-like tokens (as printed
// by compilers, test runners, git, ...) open in Crowbar's editor. One tokenizer
// reads the logical line, so a soft-wrapped link is one link on every row.
import type { IBuffer, IBufferCell, IBufferLine, IDisposable, ILink, Terminal } from '@xterm/xterm'
import { revealInEditor } from '@/features/editor/lib/reveal'
import { openExternalUrl } from '@/lib/external-open'
import { workspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { toast } from '@/features/window/stores/toast-store'
import { useTerminalStore } from '../stores/terminal-store'

/** `start`/`end` index the logical line's text; `end` is exclusive. */
type LinkToken =
  | { kind: 'url'; text: string; start: number; end: number }
  | { kind: 'file'; text: string; path: string; line?: number; start: number; end: number }

// Ported from xterm.js's web-links addon 0.12.0 (strictUrlRegex): a URL never ends
// on punctuation or a bracket, so "(https://x.dev/a)." links only the URL.
const URL_RE = /(https?|HTTPS?):[/]{2}[^\s"'!*(){}|\\^<>`]*[^\s"':,.!?{}|\\^~[\]`()<>]/g

// A path is rooted or has an inner slash, plus an optional :line[:col]. Bare
// filenames are skipped: with no filesystem check, most are domains or packages.
const FILE_LINK_RE =
  /(?:^|[\s"'`([<])((?:~\/|\.{1,2}\/|\/)?[\w.@+~-]+(?:\/[\w.@+~-]+)+\/?|\/[\w.@+~-]+)((?::\d+){1,2})?/g

// "." is a path character, so a sentence ending on a path ("see design.md.")
// would link its period; a real path never ends in a dot.
const TRAILING_DOTS_RE = /\.+$/

// How far the logical line is read above and below the hovered row.
const MAX_WALK_CELLS = 2048

// The web-links addon's isUrl: the match must parse as a URL and start with its
// own origin, so a mangled authority never becomes a link.
function isUrl(text: string): boolean {
  try {
    const url = new URL(text)
    const auth = url.username ? `${url.username}${url.password ? `:${url.password}` : ''}@` : ''
    return text.toLowerCase().startsWith(`${url.protocol}//${auth}${url.host}`.toLowerCase())
  } catch {
    return false
  }
}

// A non-space run from any scheme or "www." on is address text: a path in it
// ("?q=(web/a.ts)", "ftp://h/a/b") is never a file reference, even past the URL token.
const ADDRESS_RUN_RE = /(?:[a-zA-Z][a-zA-Z0-9+.-]*:\/\/|\bwww\.)\S*/g

function tokenizeLinks(text: string): LinkToken[] {
  const urls: LinkToken[] = []
  for (const m of text.matchAll(URL_RE)) {
    if (!isUrl(m[0])) continue
    urls.push({ kind: 'url', text: m[0], start: m.index, end: m.index + m[0].length })
  }
  const addressRuns = [...text.matchAll(ADDRESS_RUN_RE)].map((m) => ({
    start: m.index,
    end: m.index + m[0].length,
  }))
  const files: LinkToken[] = []
  for (const m of text.matchAll(FILE_LINK_RE)) {
    const path = m[1].replace(TRAILING_DOTS_RE, '')
    if (!path) continue
    const suffix = m[2] ?? ''
    const start = m.index + m[0].indexOf(m[1])
    const end = start + path.length + suffix.length
    if (addressRuns.some((run) => start < run.end && run.start < end)) continue
    const line = suffix ? Number(suffix.slice(1).split(':')[0]) : undefined
    files.push({ kind: 'file', text: path + suffix, path, line, start, end })
  }
  return [...urls, ...files]
}

// Resolves a matched path against the terminal's working root. "~" cannot be
// expanded client-side, so those candidates resolve only if the root is home
// — skip them instead of opening a wrong file.
function resolveFileLinkPath(path: string, root: string | undefined): string | null {
  if (path.startsWith('~')) return null
  if (path.startsWith('/')) return path
  if (!root || !root.startsWith('/')) return null
  const parts: string[] = root.split('/')
  for (const seg of path.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (parts.length > 1) parts.pop()
      continue
    }
    parts.push(seg)
  }
  return parts.join('/')
}

/** The cell a UTF-16 unit of the logical line came from; `x`/`y` are 0-based. */
interface CellRef {
  x: number
  y: number
  width: number
}

interface LogicalLine {
  text: string
  cells: CellRef[]
}

/** Row `y`'s logical line: xterm flags each soft-wrapped continuation `isWrapped`. */
function logicalRows(buffer: IBuffer, y: number): { top: number; rows: IBufferLine[] } | undefined {
  const hovered = buffer.getLine(y)
  if (!hovered) return undefined
  const rows = [hovered]
  let top = y
  for (let walked = 0; rows[0].isWrapped && walked < MAX_WALK_CELLS;) {
    const above = buffer.getLine(top - 1)
    if (!above) break
    rows.unshift(above)
    top--
    walked += above.length
  }
  for (let walked = 0; walked < MAX_WALK_CELLS;) {
    const below = buffer.getLine(top + rows.length)
    if (!below?.isWrapped) break
    rows.push(below)
    walked += below.length
  }
  return { top, rows }
}

// Unwritten cells read back as ''. They count as spaces only between content, so
// trailing blanks and the gap a wide glyph leaves at a wrap never split a token.
function appendRow(line: LogicalLine, row: IBufferLine, y: number, cell: IBufferCell): void {
  let blanks: CellRef[] = []
  for (let x = 0; x < row.length; x++) {
    if (!row.getCell(x, cell)) break
    const width = cell.getWidth()
    if (width === 0) continue
    const chars = cell.getChars()
    if (chars === '') {
      blanks.push({ x, y, width })
      continue
    }
    for (const blank of blanks) {
      line.text += ' '
      line.cells.push(blank)
    }
    blanks = []
    line.text += chars
    for (let unit = 0; unit < chars.length; unit++) line.cells.push({ x, y, width })
  }
}

function readLogicalLine(buffer: IBuffer, y: number): LogicalLine | undefined {
  const found = logicalRows(buffer, y)
  if (!found) return undefined
  const line: LogicalLine = { text: '', cells: [] }
  const cell = buffer.getNullCell()
  found.rows.forEach((row, i) => appendRow(line, row, found.top + i, cell))
  return line
}

// xterm ranges are 1-based and inclusive; a wide glyph's range ends on its last cell.
function rangeOf(cells: CellRef[], token: LinkToken): ILink['range'] {
  const first = cells[token.start]
  const last = cells[token.end - 1]
  return {
    start: { x: first.x + 1, y: first.y + 1 },
    end: { x: last.x + last.width, y: last.y + 1 },
  }
}

export interface TerminalLinksOptions {
  /** The directory relative paths resolve against (terminal cwd / worktree). */
  getRoot: () => string | undefined
  /** Opens the resolved absolute path in Crowbar's editor. */
  openFile: (absolutePath: string, line?: number) => void
  /** Opens a URL outside Crowbar. */
  openUrl: (uri: string) => void
  /** Called when a clicked candidate cannot be resolved to an absolute path. */
  onUnresolved?: (candidateText: string) => void
}

function activate(token: LinkToken, options: TerminalLinksOptions): void {
  if (token.kind === 'url') {
    options.openUrl(token.text)
    return
  }
  const resolved = resolveFileLinkPath(token.path, options.getRoot())
  if (resolved) options.openFile(resolved, token.line)
  else options.onUnresolved?.(token.text)
}

// Registers the terminal's one link provider. It is owned by the terminal
// instance and is disposed with it.
export function registerTerminalLinks(
  terminal: Pick<Terminal, 'buffer' | 'registerLinkProvider'>,
  options: TerminalLinksOptions,
): IDisposable {
  return terminal.registerLinkProvider({
    provideLinks(bufferLineNumber: number, callback: (links: ILink[] | undefined) => void) {
      const line = readLogicalLine(terminal.buffer.active, bufferLineNumber - 1)
      if (!line) {
        callback(undefined)
        return
      }
      const links: ILink[] = []
      for (const token of tokenizeLinks(line.text)) {
        const range = rangeOf(line.cells, token)
        if (range.start.y > bufferLineNumber || range.end.y < bufferLineNumber) continue
        links.push({ text: token.text, range, activate: () => activate(token, options) })
      }
      callback(links.length > 0 ? links : undefined)
    },
  })
}

/**
 * Links open in Crowbar. A relative path resolves against the session cwd, else
 * the terminal's own workspace root; a terminal of no workspace opens absolute paths only.
 */
export function terminalLinksFor(
  getSessionId: () => string,
  getWorkspaceId: () => string | undefined,
): TerminalLinksOptions {
  return {
    getRoot: () => {
      const cwd = useTerminalStore.getState().getSession(getSessionId())?.currentDirectory
      if (cwd?.startsWith('/')) return cwd
      const workspaceId = getWorkspaceId()
      return workspaceId ? workspaceRootPath(workspaceId) : undefined
    },
    openFile: (absolutePath, line) => {
      const position = line ? { line: line - 1, character: 0 } : undefined
      void revealInEditor({ doc: { absolutePath }, position })
    },
    // No confirmation step: a click goes straight to the default browser.
    openUrl: (uri) => {
      openExternalUrl(uri).catch((error) => console.error('Failed to open link:', error))
    },
    onUnresolved: (candidateText) => {
      toast.error('Cannot open file', `Could not resolve ${candidateText} to a path.`)
    },
  }
}
