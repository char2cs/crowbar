/** Terminal control sequences (colour, cursor) a tool's output carries verbatim. */
// eslint-disable-next-line no-control-regex -- ESC is exactly what this matches
const ANSI = /\u001b(?:\[[0-?]*[ -/]*[@-~]|\][^\u0007\u001b]*(?:\u0007|\u001b\\)|[@-Z\\-_])/g

const stripAnsi = (text: string) => text.replace(ANSI, '')

const INDENT = '  '

function scalar(value: unknown): string {
  if (typeof value === 'string') return value === '' ? '""' : stripAnsi(value)
  return JSON.stringify(value) ?? String(value)
}

/** One value after `head` at `depth`: nested values and multi-line strings go
 *  on their own indented lines, so output reads as it printed, not as `\n`s. */
function entry(head: string, value: unknown, depth: number): string[] {
  const pad = INDENT.repeat(depth)
  if (typeof value === 'string' && value.includes('\n')) {
    const lines = stripAnsi(value).replace(/\n$/, '').split('\n')
    return [`${pad}${head} |`, ...lines.map((line) => `${pad}${INDENT}${line}`)]
  }
  if (isFilled(value)) return [`${pad}${head}`, ...block(value, depth + 1)]
  return [`${pad}${head} ${scalar(value)}`]
}

const isFilled = (value: unknown): value is object =>
  value !== null && typeof value === 'object' && Object.keys(value).length > 0

function block(value: object, depth: number): string[] {
  if (Array.isArray(value)) return value.flatMap((item) => entry('-', item, depth))
  return Object.entries(value).flatMap(([key, item]) => entry(`${key}:`, item, depth))
}

/** A tool's request or result as a person reads it: JSON laid out by key with
 *  its strings unescaped, anything else as the bytes arrived; never colour codes. */
export function payloadText(text: string): string {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return stripAnsi(text) // not JSON: the provider's bytes, as they arrived
  }
  if (isFilled(value)) return block(value, 0).join('\n')
  return typeof value === 'string' ? stripAnsi(value) : text
}
