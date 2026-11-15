/** What the shell saved for a file pane: a workspace file's relative path, an absolute path the user picked on the host, or a new untitled file. */
export type EditorTarget =
  { kind: 'workspace'; path: string } | { kind: 'host'; path: string } | { kind: 'untitled' }

/** Reads a file pane's saved state; null when it names nothing to open. */
export function editorTarget(saved: unknown): EditorTarget | null {
  if (typeof saved !== 'object' || saved === null) return null
  const { path, absolute, untitled } = saved as Record<string, unknown>
  if (untitled === true) return { kind: 'untitled' }
  if (typeof path !== 'string' || path === '') return null
  return absolute === true ? { kind: 'host', path } : { kind: 'workspace', path }
}
