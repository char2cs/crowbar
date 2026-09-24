const PREFIX = 'crowbar://editor/'

/**
 * Stable Monaco model URI for a file, keyed by WORKSPACE + file path so the
 * same file in the same workspace shares one model across panes (two panes
 * on the same workspace showing the same file get live sync + shared undo —
 * the original point of keying by path at all).
 *
 * Monaco models are a single GLOBAL table (`monaco.editor.getModel`/
 * `createModel`), not scoped per `ModelRegistry` instance — keying by path
 * alone made two DIFFERENT workspaces' files at the same relative path
 * (e.g. `src/inventory.ts` in two branches of the same repo, each its own
 * worktree per the workspace model) collide on one shared model. One
 * workspace's edits, close, or disposal then landed on — or yanked out from
 * under — the other's pane: live-reported as syntax highlighting flashing
 * across unrelated panes and intermittent "Model is disposed!" errors with
 * no app-level stack, root-caused here once `data-uri` on two panes showing
 * unrelated workspaces' files came back byte-for-byte identical.
 *
 * Encodes both segments to survive spaces/unicode; the workspace segment is
 * encoded too so it can never itself contain the `/` this function uses as
 * the separator between the two encoded segments.
 */
export function fileUri(workspaceId: string, fsPath: string): string {
  return PREFIX + encodeURIComponent(workspaceId) + '/' + encodeURIComponent(fsPath)
}

/** Workspace id half of a {@link fileUri}; null for any other uri. */
export function uriToWorkspaceId(uri: string): string | null {
  if (!uri.startsWith(PREFIX)) return null
  const rest = uri.slice(PREFIX.length)
  const separatorIndex = rest.indexOf('/')
  if (separatorIndex <= 0) return null
  return decodeURIComponent(rest.slice(0, separatorIndex))
}

export function uriToFsPath(uri: string): string {
  const rest = uri.slice(PREFIX.length)
  const separatorIndex = rest.indexOf('/')
  const encodedPath = separatorIndex === -1 ? '' : rest.slice(separatorIndex + 1)
  return decodeURIComponent(encodedPath)
}
