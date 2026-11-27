import { diffLines } from '@/features/agent/composer/lib/line-diff'

export interface AskRequest {
  toolName?: string
  toolTarget?: string
  toolInput?: unknown
  truncated?: boolean
  mcpServer?: string
  why?: string
}

export interface CodeLine {
  text: string
  key?: string
  tone?: 'del' | 'add' | 'ctx'
  n?: number
  mark?: [number, number]
  cut?: boolean
}

export interface RequestView {
  server?: string
  head?: string
  code: CodeLine[]
  markdown?: string
  note?: string
  shell?: boolean
}

const SHOWN_LINES = 12
const LINE_CHARS = 240

const str = (value: unknown) => (typeof value === 'string' ? value : '')
const lines = (text: string) => text.split(/\r?\n/)
const plain = (text: string): CodeLine[] => lines(text).map((line) => ({ text: line }))

const argText = (value: unknown) => (typeof value === 'string' ? value : JSON.stringify(value))

function mcpArgs(input: unknown): CodeLine[] {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return []
  return Object.entries(input as Record<string, unknown>).flatMap(([key, v]) =>
    plain(argText(v)).map((line, i) => (i === 0 ? { ...line, key } : line)),
  )
}

function mcpView(req: AskRequest): RequestView {
  const server = req.mcpServer ?? ''
  const prefix = `mcp__${server}__`
  const name = req.toolName ?? ''
  const tool = name.startsWith(prefix) ? name.slice(prefix.length) : name
  return { server, head: tool, code: mcpArgs(req.toolInput) }
}

const toned = (text: string, tone: CodeLine['tone']): CodeLine[] =>
  text ? lines(text).map((line) => ({ text: line, tone })) : []

const replacement = (edit: Record<string, unknown>) => {
  const [before, after] = [str(edit.old_string), str(edit.new_string)]
  const diff = diffLines(before, after)
  return diff.length > 0 || before === after
    ? diff
    : [...toned(before, 'del'), ...toned(after, 'add')]
}

function fileView(input: Record<string, unknown>, target: string): RequestView | null {
  const head = target || str(input.file_path) || str(input.notebook_path) || undefined
  if (Array.isArray(input.edits)) {
    return { head, code: (input.edits as Record<string, unknown>[]).flatMap(replacement) }
  }
  if ('old_string' in input || 'new_string' in input) {
    const note = input.replace_all === true ? 'Every occurrence' : undefined
    return { head, code: replacement(input), note }
  }
  const content = str(input.content) || str(input.new_source)
  return content ? { head, code: plain(content) } : null
}

export function requestView(req: AskRequest): RequestView {
  if (req.truncated) return { code: plain(str(req.toolInput)), note: 'Input truncated' }
  if (req.mcpServer) return mcpView(req)
  const input = (req.toolInput ?? {}) as Record<string, unknown>
  const command = str(input.command)
  if (command) return { code: plain(command), shell: true }
  const plan = str(input.plan)
  if (plan) return { code: [], markdown: plan }
  return (
    fileView(input, req.toolTarget ?? '') ?? {
      head: req.toolTarget || undefined,
      code: [],
      note: req.why || 'Input not shown',
    }
  )
}

const HIDDEN = /(?![\t\n])[\p{Default_Ignorable_Code_Point}\p{Cf}\p{Zl}\p{Zp}\p{Cc}]/gu

export function visibleParts(text: string): { text: string; escaped: boolean }[] {
  const parts: { text: string; escaped: boolean }[] = []
  let last = 0
  for (const match of text.matchAll(HIDDEN)) {
    if (match.index > last) parts.push({ text: text.slice(last, match.index), escaped: false })
    const cp = match[0].codePointAt(0)!
    parts.push({ text: `\\u{${cp.toString(16).toUpperCase()}}`, escaped: true })
    last = match.index + match[0].length
  }
  if (last < text.length) parts.push({ text: text.slice(last), escaped: false })
  return parts
}

export interface FoldedCode {
  lines: CodeLine[]
  moreLines: number
  moreChars: number
}

function cutLine(line: CodeLine, max: number): CodeLine {
  let end = max
  const unit = line.text.charCodeAt(end - 1)
  if (unit >= 0xd800 && unit <= 0xdbff) end -= 1
  return { ...line, text: line.text.slice(0, end), cut: true }
}

export function foldCode(code: CodeLine[]): FoldedCode {
  let moreChars = 0
  const lines = code.slice(0, SHOWN_LINES).map((line) => {
    if (line.text.length <= LINE_CHARS) return line
    const cut = cutLine(line, LINE_CHARS)
    moreChars += line.text.length - cut.text.length
    return cut
  })
  return { lines, moreLines: code.length - lines.length, moreChars }
}

const count = (n: number, noun: string) => `${n} more ${noun}${n === 1 ? '' : 's'}`

export function foldedCue({ moreLines, moreChars }: FoldedCode): string {
  const parts = [moreLines && count(moreLines, 'line'), moreChars && count(moreChars, 'character')]
  const said = parts.filter(Boolean).join(' and ')
  return said && `${said} not shown`
}

export function showAllLabel(code: CodeLine[], folded: FoldedCode): string {
  if (folded.moreLines > 0) return `Show all ${code.length} lines`
  const chars = code.reduce((sum, line) => sum + line.text.length, 0)
  return `Show all ${chars} characters`
}
