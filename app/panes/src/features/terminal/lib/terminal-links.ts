import type { IBuffer, IBufferCell, IBufferLine, IDisposable, ILink, Terminal } from '@xterm/xterm'
import { revealInEditor } from '@/features/editor/lib/reveal'
import { LOCAL_HOST } from '@/features/hosts/types'
import { workspaceRef, type DocumentTarget } from '@/lib/api/documents'
import { resolvedHostOf } from '@/lib/host-resolve'
import { workspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { toast } from '@/features/window/stores/toast-store'
import { IS_MAC } from '@/utils/platform'
import { useTerminalStore } from '../stores/terminal-store'
import { showTerminalUrlMenu, type TerminalView } from './terminal-url-menu'

type LinkToken =
  | { kind: 'url'; text: string; start: number; end: number }
  | { kind: 'file'; text: string; path: string; line?: number; start: number; end: number }

const URL_RE = /(https?|HTTPS?):[/]{2}[^\s"'!*(){}|\\^<>`]*[^\s"':,.!?{}|\\^~[\]`()<>]/g

const FILE_LINK_RE =
  /(?:^|[\s"'`([<])((?:~\/|\.{1,2}\/|\/)?[\w.@+~-]+(?:\/[\w.@+~-]+)+\/?|\/[\w.@+~-]+)((?::\d+){1,2})?/g

const TRAILING_DOTS_RE = /\.+$/

const MAX_WALK_CELLS = 2048

function isUrl(text: string): boolean {
  try {
    const url = new URL(text)
    const auth = url.username ? `${url.username}${url.password ? `:${url.password}` : ''}@` : ''
    return text.toLowerCase().startsWith(`${url.protocol}//${auth}${url.host}`.toLowerCase())
  } catch {
    return false
  }
}

const ADDRESS_RUN_RE = /(?:[a-zA-Z][a-zA-Z0-9+.-]*:\/\/|(?<=^|[\s"'`([<])www\.)\S*/g

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
    if (addressRuns.some((run) => run.start <= start && start < run.end)) continue
    const line = suffix ? Number(suffix.slice(1).split(':')[0]) : undefined
    files.push({ kind: 'file', text: path + suffix, path, line, start, end })
  }
  return [...urls, ...files]
}

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

interface CellRef {
  x: number
  y: number
  width: number
}

interface LogicalLine {
  text: string
  cells: CellRef[]
}

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

function rangeOf(cells: CellRef[], token: LinkToken): ILink['range'] {
  const first = cells[token.start]
  const last = cells[token.end - 1]
  return {
    start: { x: first.x + 1, y: first.y + 1 },
    end: { x: last.x + last.width, y: last.y + 1 },
  }
}

export interface TerminalLinksOptions {
  getRoot: () => string | undefined
  openFile: (absolutePath: string, line?: number) => void
  onUrl: (url: string, position: { x: number; y: number }) => void
  onUnresolved?: (candidateText: string) => void
}

function isPrimaryModifier(event: { metaKey: boolean; ctrlKey: boolean }): boolean {
  return IS_MAC ? event.metaKey : event.ctrlKey
}

function activate(token: LinkToken, event: MouseEvent, options: TerminalLinksOptions): void {
  if (!isPrimaryModifier(event)) return
  if (token.kind === 'url') {
    options.onUrl(token.text, { x: event.clientX, y: event.clientY })
    return
  }
  const resolved = resolveFileLinkPath(token.path, options.getRoot())
  if (resolved) options.openFile(resolved, token.line)
  else options.onUnresolved?.(token.text)
}

function trackModifier(element: HTMLElement | undefined) {
  let held = false
  let hovered: ILink | null = null
  const set = (next: boolean) => {
    held = next
    if (!hovered?.decorations) return
    hovered.decorations.underline = next
    hovered.decorations.pointerCursor = next
  }
  const onModifierEvent = (event: KeyboardEvent | MouseEvent) => set(isPrimaryModifier(event))
  const onBlur = () => set(false)
  const events = ['keydown', 'keyup', 'mousemove'] as const
  for (const type of events) element?.addEventListener(type, onModifierEvent)
  window.addEventListener('blur', onBlur)
  return {
    decorations: () => ({ underline: held, pointerCursor: held }),
    hover: (link: ILink, event: MouseEvent) => {
      hovered = link
      set(isPrimaryModifier(event))
    },
    leave: (link: ILink) => {
      if (hovered === link) hovered = null
    },
    dispose: () => {
      for (const type of events) element?.removeEventListener(type, onModifierEvent)
      window.removeEventListener('blur', onBlur)
    },
  }
}

export function registerTerminalLinks(
  terminal: Pick<Terminal, 'buffer' | 'registerLinkProvider' | 'element' | 'options'>,
  options: TerminalLinksOptions,
): IDisposable {
  const modifier = trackModifier(terminal.element)
  terminal.options.linkHandler = {
    activate: (event, url) => {
      if (isPrimaryModifier(event)) options.onUrl(url, { x: event.clientX, y: event.clientY })
    },
  }
  const provider = terminal.registerLinkProvider({
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
        const link: ILink = {
          text: token.text,
          range,
          decorations: modifier.decorations(),
          activate: (event) => activate(token, event, options),
          hover: (event) => modifier.hover(link, event),
          leave: () => modifier.leave(link),
        }
        links.push(link)
      }
      callback(links.length > 0 ? links : undefined)
    },
  })
  return {
    dispose() {
      provider.dispose()
      modifier.dispose()
      terminal.options.linkHandler = null
    },
  }
}

function relativeTo(absolutePath: string, root: string | undefined): string | null {
  if (!root) return null
  const prefix = root.endsWith('/') ? root : `${root}/`
  return absolutePath.startsWith(prefix) ? absolutePath.slice(prefix.length) || null : null
}

function documentFor(absolutePath: string, workspaceId: string | undefined): DocumentTarget | null {
  if (!workspaceId) return { absolutePath }
  const rel = relativeTo(absolutePath, workspaceRootPath(workspaceId))
  if (rel) return workspaceRef(workspaceId, rel)
  return resolvedHostOf({ wsId: workspaceId }) === LOCAL_HOST.id ? { absolutePath } : null
}

export function terminalLinksFor(
  getSessionId: () => string,
  getView: () => Partial<TerminalView>,
): TerminalLinksOptions {
  return {
    getRoot: () => {
      const cwd = useTerminalStore.getState().getSession(getSessionId())?.currentDirectory
      if (cwd?.startsWith('/')) return cwd
      const { workspaceId } = getView()
      return workspaceId ? workspaceRootPath(workspaceId) : undefined
    },
    openFile: (absolutePath, line) => {
      const doc = documentFor(absolutePath, getView().workspaceId)
      if (!doc) {
        toast.error('Cannot open file', `${absolutePath} is outside the current workspace.`)
        return
      }
      const position = line ? { line: line - 1, character: 0 } : undefined
      void revealInEditor({ doc, position })
    },
    onUrl: (url, position) => {
      const { workspaceId, paneId } = getView()
      if (workspaceId) void showTerminalUrlMenu(url, position, { workspaceId, paneId })
    },
    onUnresolved: (candidateText) => {
      toast.error('Cannot open file', `Could not resolve ${candidateText} to a path.`)
    },
  }
}
