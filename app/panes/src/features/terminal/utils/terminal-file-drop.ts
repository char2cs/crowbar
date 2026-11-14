function quoteTerminalPath(path: string): string {
  const escaped = path.replace(/(["\\$`])/g, '\\$1')
  return /[\s"'\\$`]/.test(path) ? `"${escaped}"` : escaped
}

function isAbsolutePath(entry: string): boolean {
  return entry.startsWith('/')
}

function decodeFileUrl(entry: string): string | null {
  try {
    const url = new URL(entry)
    return url.protocol === 'file:' ? decodeURIComponent(url.pathname) : null
  } catch {
    return null // not a URL
  }
}

export function formatDroppedPathsForTerminal(rawPaths: string[]): string {
  if (rawPaths.length === 0) return ''
  const resolved: string[] = []
  for (const entry of rawPaths) {
    const fileUrlPath = decodeFileUrl(entry)
    if (fileUrlPath !== null) {
      resolved.push(fileUrlPath)
      continue
    }
    if (isAbsolutePath(entry)) {
      resolved.push(entry)
      continue
    }
    // Skip relative paths, http/https URLs, and other unsupported entries
  }
  if (resolved.length === 0) return ''
  return `${resolved.map(quoteTerminalPath).join(' ')} `
}
