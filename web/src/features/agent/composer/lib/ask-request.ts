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
}

/** The card's body: a heading (an MCP server, a path), the lines, a note. */
export interface RequestView {
  server?: string
  head?: string
  code: CodeLine[]
  note?: string
}

/** Lines shown before the card says how many more there are. */
export const SHOWN_LINES = 12

const MCP_ARGS = 6
const ARG_CHARS = 80

const str = (value: unknown) => (typeof value === 'string' ? value : '')
const plain = (text: string): CodeLine[] => text.split('\n').map((line) => ({ text: line }))

function argText(value: unknown): string {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  return text.length > ARG_CHARS ? `${text.slice(0, ARG_CHARS)}…` : text
}

/** An MCP call's arguments, one `key: value` line each, the first few only. */
function mcpArgsSummary(input: unknown): string[] {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return []
  const entries = Object.entries(input as Record<string, unknown>)
  const lines = entries.slice(0, MCP_ARGS).map(([key, value]) => `${key}: ${argText(value)}`)
  if (entries.length > MCP_ARGS) lines.push(`+${entries.length - MCP_ARGS} more`)
  return lines
}

function mcpView(req: AskRequest): RequestView {
  const server = req.mcpServer ?? ''
  const prefix = `mcp__${server}__`
  const name = req.toolName ?? ''
  const tool = name.startsWith(prefix) ? name.slice(prefix.length) : name
  return { server, head: tool, code: mcpArgsSummary(req.toolInput).map((text) => ({ text })) }
}

/** The body of a permission card, by what the gated call is; null for none. */
export function requestView(req: AskRequest): RequestView | null {
  // A truncated input is the leading text of its JSON: shown, never parsed.
  if (req.truncated) return { code: plain(str(req.toolInput)), note: 'Input truncated' }
  if (req.mcpServer) return mcpView(req)
  const input = (req.toolInput ?? {}) as Record<string, unknown>
  const command = str(input.command)
  if (command) return { code: plain(command), note: str(input.description) || undefined }
  const removed = str(input.old_string)
  const added = str(input.new_string)
  if (removed || added) {
    const head = str(input.file_path) || req.toolTarget || undefined
    const lines = (text: string, tone: CodeLine['tone']) =>
      text.split('\n').map((line) => ({ text: line, tone }))
    return { head, code: [...lines(removed, 'del'), ...lines(added, 'add')] }
  }
  return null
}

// Bidi controls, other control characters and zero-width ones: drawn raw they
// reorder or hide what the command does, so each is written out instead.
const HIDDEN_RANGES: [number, number][] = [
  [0x00, 0x08],
  [0x0b, 0x1f],
  [0x7f, 0x9f],
  [0x061c, 0x061c],
  [0x200b, 0x200f],
  [0x202a, 0x202e],
  [0x2060, 0x2069],
  [0xfeff, 0xfeff],
]

const hidden = (cp: number) => HIDDEN_RANGES.some(([lo, hi]) => cp >= lo && cp <= hi)

/** `text` split into what draws as itself and the characters written out. */
export function visibleParts(text: string): { text: string; escaped: boolean }[] {
  const parts: { text: string; escaped: boolean }[] = []
  let run = ''
  for (const ch of text) {
    const cp = ch.codePointAt(0)!
    if (!hidden(cp)) {
      run += ch
      continue
    }
    if (run) parts.push({ text: run, escaped: false })
    run = ''
    parts.push({ text: `\\u{${cp.toString(16).toUpperCase()}}`, escaped: true })
  }
  if (run) parts.push({ text: run, escaped: false })
  return parts
}
