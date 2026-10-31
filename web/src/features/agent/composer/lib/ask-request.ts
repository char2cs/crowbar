/** What a permission asks to run, from the ask's own copy of the call's input. */
export interface AskRequest {
  toolName?: string
  toolTarget?: string
  /** The call's input; with `truncated`, a string holding its leading text. */
  toolInput?: unknown
  truncated?: boolean
  mcpServer?: string
}

export type RequestBody =
  | { kind: 'truncated'; text: string }
  | { kind: 'mcp'; server: string; tool: string; args: string[] }
  | { kind: 'command'; command: string; description: string }
  | { kind: 'edit'; path: string; removed: string[]; added: string[] }
  | { kind: 'none' }

const MCP_ARGS = 6
const ARG_CHARS = 80

const str = (value: unknown) => (typeof value === 'string' ? value : '')

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

/** The body of a permission card, by what the gated call is. */
export function requestBody(req: AskRequest): RequestBody {
  // A truncated input is the leading text of its JSON: shown, never parsed.
  if (req.truncated) return { kind: 'truncated', text: str(req.toolInput) }
  const input = (req.toolInput ?? {}) as Record<string, unknown>
  if (req.mcpServer) {
    const prefix = `mcp__${req.mcpServer}__`
    const name = req.toolName ?? ''
    const tool = name.startsWith(prefix) ? name.slice(prefix.length) : name
    return { kind: 'mcp', server: req.mcpServer, tool, args: mcpArgsSummary(req.toolInput) }
  }
  const command = str(input.command)
  if (command) return { kind: 'command', command, description: str(input.description) }
  const removed = str(input.old_string)
  const added = str(input.new_string)
  if (removed || added) {
    const path = str(input.file_path) || req.toolTarget || ''
    return { kind: 'edit', path, removed: removed.split('\n'), added: added.split('\n') }
  }
  return { kind: 'none' }
}
