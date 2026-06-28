/**
 * Parse OSC 7 sequence for working directory tracking.
 * OSC 7 format: ESC]7;file://hostname/pathBEL
 * Returns the directory from the LAST match in `data` — important for the
 * reconnect replay burst, where the whole ring (many prompts) arrives at once
 * and the newest cwd must win.
 */
export function parseOSC7(data: string): string | null {
  const ESC = String.fromCharCode(0x1b)
  const BEL = String.fromCharCode(0x07)
  const osc7Regex = new RegExp(`${ESC}\\]7;file://[^/]*([^${BEL}]+)${BEL}`, 'g')

  let lastPath: string | null = null
  for (const match of data.matchAll(osc7Regex)) {
    if (match[1]) lastPath = match[1]
  }
  if (lastPath === null) return null
  try {
    return decodeURIComponent(lastPath)
  } catch {
    return lastPath
  }
}
