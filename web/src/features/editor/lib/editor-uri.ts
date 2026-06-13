const PREFIX = 'athas://editor/'

/** Stable Monaco model URI for a file, keyed by file path so the same file
 *  shares one model across panes. Encodes the path to survive spaces/unicode. */
export function fileUri(fsPath: string): string {
  return PREFIX + encodeURIComponent(fsPath)
}

export function uriToFsPath(uri: string): string {
  return decodeURIComponent(uri.slice(PREFIX.length))
}
