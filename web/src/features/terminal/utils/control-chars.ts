/**
 * Strip C0 control bytes (0x00–0x1F), DEL (0x7F) and the C1 CSI introducer
 * (0x9B) from a string.
 *
 * Terminal-sourced strings that end up in the session store (OSC 7 paths,
 * OSC 0/1/2 titles, user-entered names) must be control-free: downstream,
 * useBufferDisplayName projects session fields into \x01-joined tuples for
 * its narrowed store subscription, so a control byte that survives ingress
 * could truncate/alias those tuples and corrupt tab labels. Same byte set as
 * sanitizeTerminalTitle has always dropped — this is that loop, extracted.
 */
export function stripControlChars(input: string): string {
  let out = ''
  for (const ch of input) {
    const code = ch.charCodeAt(0)
    if (code <= 31 || code === 127 || code === 155) continue
    out += ch
  }
  return out
}
