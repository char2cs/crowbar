const SLASH = /^\/([A-Za-z][\w:-]*)(?:[ \t]+(.*))?$/

export function slashCommand(text: string): { command: string; args: string } | undefined {
  const match = SLASH.exec(text.trim())
  if (!match) return undefined
  return { command: `/${match[1]}`, args: match[2]?.trim() ?? '' }
}
