export function stripControlChars(input: string): string {
  let out = ''
  for (const ch of input) {
    const code = ch.charCodeAt(0)
    if (code <= 31 || code === 127 || code === 155) continue
    out += ch
  }
  return out
}
