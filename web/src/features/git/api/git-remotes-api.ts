import { apiFetch } from '@/lib/api'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'

export interface GitRemoteActionResult {
  success: boolean
  error?: string
}

// Push/pull are slow git ops: the daemon accepts them (202 Accepted) and
// runs them in the background. The real outcome — new ahead/behind counts, a
// merge conflict — arrives over the git-status WebSocket stream, not this
// response. A rejected POST (4xx/5xx) means the op never started.
const gitRemoteOp = async (
  wsId: string,
  action: 'push' | 'pull',
): Promise<GitRemoteActionResult> => {
  try {
    await apiFetch(`${gitBaseForWorkspace(wsId)}/${action}`, { method: 'POST' })
    return { success: true }
  } catch (error) {
    console.error(`Failed to ${action}:`, error)
    return { success: false, error: error instanceof Error ? error.message : String(error) }
  }
}

export const pushChanges = (wsId: string): Promise<GitRemoteActionResult> =>
  gitRemoteOp(wsId, 'push')

export const pullChanges = (wsId: string): Promise<GitRemoteActionResult> =>
  gitRemoteOp(wsId, 'pull')
