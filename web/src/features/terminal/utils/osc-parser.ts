const ESC = String.fromCharCode(0x1b)
const BEL = String.fromCharCode(0x07)
const OSC7_PREFIX = `${ESC}]7;`
// Compiled once at module load: parseOSC7 runs on EVERY output-flush frame while
// a terminal streams, so recompiling this per call (the old behavior) burned CPU
// on the hot path. matchAll() clones the regex internally, so reuse is safe.
const OSC7_REGEX = new RegExp(`${ESC}\\]7;file://[^/]*([^${BEL}]+)${BEL}`, 'g')

/**
 * Parse OSC 7 sequence for working directory tracking.
 * OSC 7 format: ESC]7;file://hostname/pathBEL
 * Returns the directory from the LAST match in `data` — important for the
 * reconnect replay burst, where the whole ring (many prompts) arrives at once
 * and the newest cwd must win.
 */
export function parseOSC7(data: string): string | null {
  // Fast path: terminal output rarely contains OSC 7 (only at a shell prompt),
  // but this runs on every streamed frame. A native substring scan is far
  // cheaper than the regex, so bail out before matchAll on the common case.
  if (data.indexOf(OSC7_PREFIX) === -1) return null

  let lastPath: string | null = null
  for (const match of data.matchAll(OSC7_REGEX)) {
    if (match[1]) lastPath = match[1]
  }
  if (lastPath === null) return null
  try {
    return decodeURIComponent(lastPath)
  } catch {
    return lastPath
  }
}
