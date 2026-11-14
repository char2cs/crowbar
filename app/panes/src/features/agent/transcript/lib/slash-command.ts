const SLASH = /^\/([A-Za-z][\w:-]*)(?:[ \t]+(.*))?$/

/** A single-line message that opens with `/name`; a path such as `/usr/bin` is not one. */
export function slashCommand(text: string): { command: string; args: string } | undefined {
  const match = SLASH.exec(text.trim())
  if (!match) return undefined
  return { command: `/${match[1]}`, args: match[2]?.trim() ?? '' }
}
