/** What the shell saved for a file pane: a workspace file's relative path, an absolute path the user picked on the host, or a new untitled file. `fresh` marks a file the user opened just now rather than one restored with the layout. */
export type EditorTarget =
  | { kind: 'workspace'; path: string; fresh: boolean }
  | { kind: 'host'; path: string; fresh: boolean }
  | { kind: 'untitled' }

/** Reads a file pane's saved state; null when it names nothing to open. */
export function editorTarget(saved: unknown): EditorTarget | null {
  if (typeof saved !== 'object' || saved === null) return null
  const { path, absolute, untitled, fresh } = saved as Record<string, unknown>
  if (untitled === true) return { kind: 'untitled' }
  if (typeof path !== 'string' || path === '') return null
  const kind = absolute === true ? 'host' : 'workspace'
  return { kind, path, fresh: fresh === true }
}
