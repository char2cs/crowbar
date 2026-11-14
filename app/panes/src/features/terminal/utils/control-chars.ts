// Strips C0, DEL and C1 CSI. Session-store strings must be control-free: useBufferDisplayName
// joins fields with \x01, so a surviving control byte could alias tuples and corrupt tab labels.
export function stripControlChars(input: string): string {
  let out = ''
  for (const ch of input) {
    const code = ch.charCodeAt(0)
    if (code <= 31 || code === 127 || code === 155) continue
    out += ch
  }
  return out
}
