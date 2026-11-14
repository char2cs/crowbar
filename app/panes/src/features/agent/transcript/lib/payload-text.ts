// eslint-disable-next-line no-control-regex -- ESC is exactly what this matches
const ANSI = /\u001b(?:\[[0-?]*[ -/]*[@-~]|\][^\u0007\u001b]*(?:\u0007|\u001b\\)|[@-Z\\-_])/g
/** `text` without the terminal colour and cursor codes it was printed with. */
export const plain = (text: string) => text.replace(ANSI, '')
const isFilled = (value: unknown): value is object =>
  value !== null && typeof value === 'object' && Object.keys(value).length > 0

/** `value` after `head`, indented `pad`: nested values and multi-line strings take
 *  their own lines, so output reads as it printed rather than as `\n` escapes. */
function entry(head: string, value: unknown, pad: string): string[] {
  if (typeof value === 'string' && value.includes('\n'))
    return [
      `${pad}${head} |`,
      ...plain(value)
        .replace(/\n$/, '')
        .split('\n')
        .map((l) => `${pad}  ${l}`),
    ]
  if (isFilled(value)) return [`${pad}${head}`, ...block(value, `${pad}  `)]
  const text =
    typeof value === 'string' ? (value === '' ? '""' : plain(value)) : JSON.stringify(value)
  return [`${pad}${head} ${text}`]
}

const block = (value: object, pad: string): string[] =>
  Array.isArray(value)
    ? value.flatMap((item) => entry('-', item, pad))
    : Object.entries(value).flatMap(([key, item]) => entry(`${key}:`, item, pad))

/** A tool's request or result as a person reads it: JSON laid out by key with its
 *  strings unescaped, anything else as the bytes arrived; never terminal colour codes. */
export function payloadText(text: string): string {
  try {
    const value: unknown = JSON.parse(text)
    if (isFilled(value)) return block(value, '').join('\n')
    return typeof value === 'string' ? plain(value) : text
  } catch {
    return plain(text) // not JSON: the provider's bytes, as they arrived
  }
}
