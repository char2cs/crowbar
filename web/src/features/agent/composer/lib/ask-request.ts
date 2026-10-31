/** What a permission asks to run, from the ask's own copy of the call's input. */
export interface AskRequest {
  toolName?: string
  toolTarget?: string
  /** The call's input; with `truncated`, a string holding its leading text. */
  toolInput?: unknown
  truncated?: boolean
  mcpServer?: string
}

export interface CodeLine {
  text: string
  tone?: 'del' | 'add'
  /** Drawn short: the rest of the line is counted, not shown. */
  cut?: boolean
}

/** The card's body: a heading (an MCP server, a path), the lines, a note. */
export interface RequestView {
  server?: string
  head?: string
  code: CodeLine[]
  note?: string
}

/** What a folded card draws before it says how much more there is. */
const SHOWN_LINES = 12
const LINE_CHARS = 240

const str = (value: unknown) => (typeof value === 'string' ? value : '')
// A CRLF pair is one break; a lone carriage return is left for the escapes.
const lines = (text: string) => text.split(/\r?\n/)
const plain = (text: string): CodeLine[] => lines(text).map((line) => ({ text: line }))

const argText = (value: unknown) => (typeof value === 'string' ? value : JSON.stringify(value))

/** An MCP call's arguments, one `key: value` line each. */
function mcpArgs(input: unknown): string[] {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return []
  return Object.entries(input as Record<string, unknown>).map(([key, v]) => `${key}: ${argText(v)}`)
}

function mcpView(req: AskRequest): RequestView {
  const server = req.mcpServer ?? ''
  const prefix = `mcp__${server}__`
  const name = req.toolName ?? ''
  const tool = name.startsWith(prefix) ? name.slice(prefix.length) : name
  return { server, head: tool, code: mcpArgs(req.toolInput).map((text) => ({ text })) }
}

const toned = (text: string, tone: CodeLine['tone']): CodeLine[] =>
  text ? lines(text).map((line) => ({ text: line, tone })) : []

/** One replacement as removed then added lines; an empty side draws nothing. */
const replacement = (edit: Record<string, unknown>) => [
  ...toned(str(edit.old_string), 'del'),
  ...toned(str(edit.new_string), 'add'),
]

/** A file change: an edit, every edit of a multi-edit, a write, a notebook cell. */
function fileView(input: Record<string, unknown>, target: string): RequestView | null {
  const head = str(input.file_path) || str(input.notebook_path) || target || undefined
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

/** The body of a permission card, by what the gated call is. */
export function requestView(req: AskRequest): RequestView {
  // A truncated input is the leading text of its JSON: shown, never parsed.
  if (req.truncated) return { code: plain(str(req.toolInput)), note: 'Input truncated' }
  if (req.mcpServer) return mcpView(req)
  const input = (req.toolInput ?? {}) as Record<string, unknown>
  const command = str(input.command)
  if (command) return { code: plain(command), note: str(input.description) || undefined }
  return (
    fileView(input, req.toolTarget ?? '') ?? {
      head: req.toolTarget || undefined,
      code: [],
      note: 'Input not shown',
    }
  )
}

// Controls, bidi and format characters, and ones that draw as blank: shown raw
// they reorder or hide what the command does, so each is written out instead.
const HIDDEN_RANGES: [number, number][] = [
  [0x00, 0x08],
  [0x0b, 0x1f],
  [0x7f, 0x9f],
  [0xad, 0xad],
  [0x034f, 0x034f],
  [0x061c, 0x061c],
  [0x115f, 0x1160],
  [0x180e, 0x180e],
  [0x200b, 0x200f],
  [0x2028, 0x202e],
  [0x2060, 0x206f],
  [0x2800, 0x2800],
  [0x3164, 0x3164],
  [0xfe00, 0xfe0f],
  [0xfeff, 0xfeff],
  [0xffa0, 0xffa0],
  [0xfff9, 0xfffc],
  [0xe0000, 0xe007f],
  [0xe0100, 0xe01ef],
]

const HIDDEN = new RegExp(
  `[${HIDDEN_RANGES.map(([lo, hi]) => `\\u{${lo.toString(16)}}-\\u{${hi.toString(16)}}`).join('')}]`,
  'gu',
)

/** `text` split into what draws as itself and the characters written out. */
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

/** A folded card's lines, each cut to a budget, and what that leaves out. */
export interface FoldedCode {
  lines: CodeLine[]
  moreLines: number
  moreChars: number
}

/** A line cut at `max` without splitting a surrogate pair. */
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

/** What a folded card leaves out, in words; empty when it leaves nothing. */
export function foldedCue({ moreLines, moreChars }: FoldedCode): string {
  const parts = [moreLines && count(moreLines, 'line'), moreChars && count(moreChars, 'character')]
  const said = parts.filter(Boolean).join(' and ')
  return said && `${said} not shown`
}

/** The "show everything" label: by lines when lines are left out, else by characters. */
export function showAllLabel(code: CodeLine[], folded: FoldedCode): string {
  if (folded.moreLines > 0) return `Show all ${code.length} lines`
  const chars = code.reduce((sum, line) => sum + line.text.length, 0)
  return `Show all ${chars} characters`
}
