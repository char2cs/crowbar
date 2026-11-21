/** What the shell saved for a file pane: a workspace file's relative path, an absolute path the user picked on the host, or a new untitled file. `fresh` marks a file the user opened just now rather than one restored with the layout; `line` (1-based) is where such an open reveals, as a terminal's `path:line` link asks. */
export type EditorTarget =
  | { kind: 'workspace'; path: string; fresh: boolean; line: number | null }
  | { kind: 'host'; path: string; fresh: boolean; line: number | null }
  | { kind: 'untitled' }

/** Reads a file pane's saved state; null when it names nothing to open. */
export function editorTarget(saved: unknown): EditorTarget | null {
  if (typeof saved !== 'object' || saved === null) return null
  const { path, absolute, untitled, fresh, line } = saved as Record<string, unknown>
  if (untitled === true) return { kind: 'untitled' }
  if (typeof path !== 'string' || path === '') return null
  const kind = absolute === true ? 'host' : 'workspace'
  const at = typeof line === 'number' && Number.isInteger(line) && line > 0 ? line : null
  return { kind, path, fresh: fresh === true, line: at }
}
